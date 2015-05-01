package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/rpc"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"time"

	dep "github.com/dvyukov/go-fuzz/go-fuzz-dep"
)

const (
	syncPeriod   = 3 * time.Second
	syncDeadline = 100 * syncPeriod

	minScore = 1.0
	maxScore = 1000.0
	defScore = 10.0
)

// Hub contains data shared between all slaves in the process (e.g. corpus).
// This reduces memory consumption for highly parallel slaves.
// Hub also handles communication with the master.
type Hub struct {
	id     int
	master *rpc.Client

	ro atomic.Value // *ROData

	maxCoverSize int
	corpus       []Input
	corpusSigs   map[Sig]struct{}
	corpusStale  bool
	triageQueue  []MasterInput

	triageC     chan MasterInput
	newInputC   chan Input
	newCrasherC chan NewCrasherArgs
	syncC       chan Stats

	stats Stats
}

type ROData struct {
	corpus       []Input
	maxCover     []byte
	badInputs    map[Sig]struct{}
	suppressions map[Sig]struct{}
	strLits      [][]byte // string literals in testee
	intLits      [][]byte // int literals in testee
}

type Stats struct {
	execs    uint64
	restarts uint64
}

func newHub() *Hub {
	procs := *flagProcs
	c, err := rpc.Dial("tcp", *flagSlave)
	if err != nil {
		log.Fatalf("failed to dial: %v", err)
	}
	var res ConnectRes
	if err := c.Call("Master.Connect", &ConnectArgs{Procs: procs}, &res); err != nil {
		log.Fatalf("failed to connect to master: %v", err)
	}

	// Fetch list of string and integer literals from testee.
	strLits, intLits := fetchLiterals()

	hub := &Hub{
		id:          res.ID,
		master:      c,
		corpusSigs:  make(map[Sig]struct{}),
		triageQueue: res.Corpus,
		triageC:     make(chan MasterInput, procs),
		newInputC:   make(chan Input, procs),
		newCrasherC: make(chan NewCrasherArgs, procs),
		syncC:       make(chan Stats, procs),
	}

	ro := &ROData{
		maxCover:     make([]byte, dep.CoverSize),
		badInputs:    make(map[Sig]struct{}),
		suppressions: make(map[Sig]struct{}),
		strLits:      strLits,
		intLits:      intLits,
	}
	hub.ro.Store(ro)

	go hub.loop()

	return hub
}

func (hub *Hub) loop() {
	// Local buffer helps to avoid deadlocks on chan overflows.
	var triageC chan MasterInput
	var triageInput MasterInput

	syncTicker := time.NewTicker(syncPeriod).C
	for {
		if len(hub.triageQueue) > 0 && triageC == nil {
			n := len(hub.triageQueue) - 1
			triageInput = hub.triageQueue[n]
			hub.triageQueue[n] = MasterInput{}
			hub.triageQueue = hub.triageQueue[:n]
			triageC = hub.triageC
		}

		select {
		case <-syncTicker:
			// Sync with the master.
			args := &SyncArgs{
				ID:            hub.id,
				Execs:         hub.stats.execs,
				Restarts:      hub.stats.restarts,
				CoverFullness: float64(hub.maxCoverSize) / dep.CoverSize,
			}
			hub.stats.execs = 0
			hub.stats.restarts = 0
			var res SyncRes
			if err := hub.master.Call("Master.Sync", args, &res); err != nil {
				log.Printf("sync call failed: %v", err)
				break
			}
			if len(res.Inputs) > 0 {
				hub.triageQueue = append(hub.triageQueue, res.Inputs...)
			}
			if hub.corpusStale {
				hub.updateScores()
				hub.corpusStale = false
			}

		case triageC <- triageInput:
			// Send new input to slaves for triage.
			if len(hub.triageQueue) > 0 {
				n := len(hub.triageQueue) - 1
				triageInput = hub.triageQueue[n]
				hub.triageQueue[n] = MasterInput{}
				hub.triageQueue = hub.triageQueue[:n]
			} else {
				triageC = nil
				triageInput = MasterInput{}
			}

		case s := <-hub.syncC:
			// Sync from a slave.
			hub.stats.execs += s.execs
			hub.stats.restarts += s.restarts

		case input := <-hub.newInputC:
			// New interesting input from slaves.
			ro := hub.ro.Load().(*ROData)
			newCover, newCount := compareCover(ro.maxCover, input.cover)
			if !newCover && !newCount {
				break
			}
			sig := hash(input.data)
			if _, ok := hub.corpusSigs[sig]; ok {
				break
			}

			// Passed deduplication, taking it.
			if *flagV >= 2 {
				log.Printf("hub received new input [%v]%v mine=%v", len(input.data), hash(input.data), input.mine)
			}
			hub.corpusSigs[sig] = struct{}{}
			ro1 := new(ROData)
			*ro1 = *ro
			// Assign it the default score, but mark corpus for score recalculation.
			hub.corpusStale = true
			scoreSum := 0
			if len(ro1.corpus) > 0 {
				scoreSum = ro1.corpus[len(ro1.corpus)-1].runningScoreSum
			}
			input.score = defScore
			input.runningScoreSum = scoreSum + defScore
			ro1.corpus = append(ro1.corpus, input)
			ro1.maxCover = make([]byte, dep.CoverSize)
			copy(ro1.maxCover, ro.maxCover)
			hub.maxCoverSize = updateMaxCover(ro1.maxCover, input.cover)
			hub.ro.Store(ro1)

			if input.mine {
				if err := hub.master.Call("Master.NewInput", NewInputArgs{hub.id, input.data, uint64(input.depth)}, nil); err != nil {
					log.Printf("new input call failed: %v", err)
				}
			}

			if *flagDumpCover {
				dumpCover(filepath.Join(*flagWorkdir, "coverprofile"), *flagBin+".cover", ro1.maxCover)
			}

		case crash := <-hub.newCrasherC:
			// New crasher from slaves. Woohoo!
			if crash.Hanging {
				ro := hub.ro.Load().(*ROData)
				ro1 := new(ROData)
				*ro1 = *ro
				if crash.Hanging {
					ro1.badInputs = make(map[Sig]struct{})
					for k, v := range ro.badInputs {
						ro1.badInputs[k] = v
					}
					ro1.badInputs[hash(crash.Data)] = struct{}{}
				}
				ro1.suppressions = make(map[Sig]struct{})
				for k, v := range ro.suppressions {
					ro1.suppressions[k] = v
				}
				ro1.suppressions[hash(crash.Suppression)] = struct{}{}
				hub.ro.Store(ro1)
			}
			if err := hub.master.Call("Master.NewCrasher", crash, nil); err != nil {
				log.Printf("new crasher call failed: %v", err)
			}
		}
	}
}

func (hub *Hub) updateScores() {
	ro := hub.ro.Load().(*ROData)
	ro1 := new(ROData)
	*ro1 = *ro
	corpus := make([]Input, len(ro.corpus))
	copy(corpus, ro.corpus)
	ro1.corpus = corpus

	var sumExecTime, sumCoverSize uint64
	for _, inp := range corpus {
		sumExecTime += inp.execTime
		sumCoverSize += uint64(inp.coverSize)
	}
	n := uint64(len(corpus))
	avgExecTime := sumExecTime / n
	avgCoverSize := sumCoverSize / n

	scoreSum := 0
	for i, inp := range corpus {
		score := defScore

		// Execution time multiplier 0.1-3x.
		// Fuzzing faster inputs increases efficiency.
		execTime := float64(inp.execTime) / float64(avgExecTime)
		if execTime > 10 {
			score /= 10
		} else if execTime > 4 {
			score /= 4
		} else if execTime > 4 {
			score /= 4
		} else if execTime > 2 {
			score /= 2
		} else if execTime < 0.25 {
			score *= 3
		} else if execTime < 0.33 {
			score *= 2
		} else if execTime < 0.5 {
			score *= 1.5
		}

		// Coverage size multiplier 0.25-3x.
		// Inputs with larger coverage are more interesting.
		coverSize := float64(inp.coverSize) / float64(avgCoverSize)
		if coverSize > 3 {
			score *= 3
		} else if coverSize > 2 {
			score *= 2
		} else if coverSize > 1.5 {
			score *= 1.5
		} else if coverSize < 0.3 {
			score /= 4
		} else if coverSize < 0.5 {
			score /= 2
		} else if coverSize < 0.75 {
			score /= 1.5
		}

		// Input depth multiplier 1-5x.
		// Deeper inputs have higher chances of digging deeper into code.
		if inp.depth < 10 {
			// no boost for you
		} else if inp.depth < 20 {
			score *= 2
		} else if inp.depth < 40 {
			score *= 3
		} else if inp.depth < 80 {
			score *= 4
		} else {
			score *= 5
		}

		// User boost (Fuzz function return value) multiplier 1-4x.
		// We don't know what it is, but user said so.
		switch inp.res {
		case 0:
			// no boost for you
		case 1:
			// Assuming this is a correct input (e.g. deserialized successfully).
			score *= 2
		default:
			// Assuming this is a correct and interesting in some way input.
			score *= 4
		}

		if score < minScore {
			score = minScore
		}
		if score > maxScore {
			score = maxScore
		}
		scoreSum += int(score)
		corpus[i].score = int(score)
		corpus[i].runningScoreSum = scoreSum
	}
	hub.ro.Store(ro1)
}

func updateMaxCover(base, cur []byte) int {
	if len(base) != dep.CoverSize || len(cur) != dep.CoverSize {
		log.Fatalf("bad cover table size (%v, %v)", len(base), len(cur))
	}
	cnt := 0
	for i, x := range cur {
		// Quantize the counters.
		// Otherwise we get too inflated corpus.
		if x == 0 {
			x = 0
		} else if x <= 1 {
			x = 1
		} else if x <= 2 {
			x = 2
		} else if x <= 4 {
			x = 4
		} else if x <= 8 {
			x = 8
		} else if x <= 16 {
			x = 16
		} else if x <= 32 {
			x = 32
		} else if x <= 64 {
			x = 64
		} else {
			x = 255
		}
		v := base[i]
		if v != 0 || x > 0 {
			cnt++
		}
		if v < x {
			base[i] = x
		}
	}
	return cnt
}

func dumpCover(outf, metaf string, cover []byte) {
	type Block struct {
		File      string
		StartLine int
		StartCol  int
		EndLine   int
		EndCol    int
		NumStmt   int
	}
	metadata, err := ioutil.ReadFile(metaf)
	if err != nil {
		log.Fatalf("failed to read coverage metadata: %v", err)
	}
	meta := make(map[string][]Block)
	if err := json.Unmarshal(metadata, &meta); err != nil {
		log.Fatalf("failed to unmarshal coverage metadata: %v", err)
	}
	out, err := os.Create(outf)
	if err != nil {
		log.Fatalf("failed to create coverage file: %v", err)
	}
	defer out.Close()
	fmt.Fprintf(out, "mode: count\n")
	for i, v := range cover {
		for _, b := range meta[strconv.Itoa(i)] {
			fmt.Fprintf(out, "%s:%v.%v,%v.%v %v %v\n",
				b.File, b.StartLine, b.StartCol, b.EndLine, b.EndCol, b.NumStmt, v)
		}
	}
}
