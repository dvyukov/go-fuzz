// Copyright 2015 go-fuzz project authors. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

package main

import (
	"fmt"
	"log"
	"net/rpc"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dvyukov/go-fuzz/go-fuzz/versifier"

	. "github.com/dvyukov/go-fuzz/go-fuzz-defs"
	. "github.com/dvyukov/go-fuzz/internal/go-fuzz-types"
)

const (
	syncPeriod             = 3 * time.Second
	syncDeadline           = 100 * syncPeriod
	connectionPollInterval = 100 * time.Millisecond

	minScore = 1.0
	maxScore = 1000.0
	defScore = 10.0
)

// Hub contains data shared between all workers in the process (e.g. corpus).
// This reduces memory consumption for highly parallel workers.
// Hub also handles communication with the coordinator.
type Hub struct {
	id          int
	coordinator *rpc.Client

	ro atomic.Value // *ROData

	maxCoverMu sync.Mutex
	maxCover   atomic.Value // []byte

	initialTriage uint32

	corpusCoverSize int
	corpusSigs      map[Sig]struct{}
	corpusStale     bool
	triageQueue     []CoordinatorInput

	triageC     chan CoordinatorInput
	newInputC   chan Input
	newCrasherC chan NewCrasherArgs
	syncC       chan Stats

	stats         Stats
	corpusOrigins [execCount]uint64
}

type ROData struct {
	corpus       []Input
	corpusCover  []byte
	badInputs    map[Sig]struct{}
	suppressions map[Sig]struct{}
	strLits      [][]byte // string literals in testee
	intLits      [][]byte // int literals in testee
	coverBlocks  map[int][]CoverBlock
	sonarSites   []SonarSite
	verse        *versifier.Verse
}

type Stats struct {
	execs    uint64
	restarts uint64
}

func newHub(metadata MetaData) *Hub {
	procs := *flagProcs
	hub := &Hub{
		corpusSigs:  make(map[Sig]struct{}),
		triageC:     make(chan CoordinatorInput, procs),
		newInputC:   make(chan Input, procs),
		newCrasherC: make(chan NewCrasherArgs, procs),
		syncC:       make(chan Stats, procs),
	}

	if err := hub.connect(); err != nil {
		log.Fatalf("failed to connect to coordinator: %v", err)
	}

	coverBlocks := make(map[int][]CoverBlock)
	for _, b := range metadata.Blocks {
		coverBlocks[b.ID] = append(coverBlocks[b.ID], b)
	}
	sonarSites := make([]SonarSite, len(metadata.Sonar))
	for i, b := range metadata.Sonar {
		if i != b.ID {
			log.Fatalf("corrupted sonar metadata")
		}
		sonarSites[i].id = b.ID
		sonarSites[i].loc = fmt.Sprintf("%v:%v.%v,%v.%v", b.File, b.StartLine, b.StartCol, b.EndLine, b.EndCol)
	}
	hub.maxCover.Store(make([]byte, CoverSize))

	ro := &ROData{
		corpusCover:  make([]byte, CoverSize),
		badInputs:    make(map[Sig]struct{}),
		suppressions: make(map[Sig]struct{}),
		coverBlocks:  coverBlocks,
		sonarSites:   sonarSites,
	}
	// Prepare list of string and integer literals.
	for _, lit := range metadata.Literals {
		if lit.IsStr {
			ro.strLits = append(ro.strLits, []byte(lit.Val))
		} else {
			ro.intLits = append(ro.intLits, []byte(lit.Val))
		}
	}
	hub.ro.Store(ro)

	go hub.loop()

	return hub
}

func (hub *Hub) connect() error {
	var c *rpc.Client
	var err error

	t := time.Now()
	for {
		c, err = rpc.Dial("tcp", *flagWorker)
		if err == nil || time.Since(t) > *flagConnectionTimeout {
			break
		}
		time.Sleep(connectionPollInterval)
	}
	if err != nil {
		return err
	}
	var res ConnectRes
	if err := c.Call("Coordinator.Connect", &ConnectArgs{Procs: *flagProcs}, &res); err != nil {
		return err
	}

	hub.coordinator = c
	hub.id = res.ID
	hub.initialTriage = uint32(len(res.Corpus))
	hub.triageQueue = res.Corpus
	return nil
}

func (hub *Hub) loop() {
	// Local buffer helps to avoid deadlocks on chan overflows.
	var triageC chan CoordinatorInput
	var triageInput CoordinatorInput

	syncTicker := time.NewTicker(syncPeriod).C
	for {
		if len(hub.triageQueue) > 0 && triageC == nil {
			n := len(hub.triageQueue) - 1
			triageInput = hub.triageQueue[n]
			hub.triageQueue[n] = CoordinatorInput{}
			hub.triageQueue = hub.triageQueue[:n]
			triageC = hub.triageC
		}

		select {
		case <-syncTicker:
			// Sync with the coordinator.
			if *flagV >= 1 {
				ro := hub.ro.Load().(*ROData)
				log.Printf("hub: corpus=%v bootstrap=%v fuzz=%v minimize=%v versifier=%v smash=%v sonar=%v",
					len(ro.corpus), hub.corpusOrigins[execBootstrap]+hub.corpusOrigins[execCorpus],
					hub.corpusOrigins[execFuzz]+hub.corpusOrigins[execSonar],
					hub.corpusOrigins[execMinimizeInput]+hub.corpusOrigins[execMinimizeCrasher],
					hub.corpusOrigins[execVersifier], hub.corpusOrigins[execSmash],
					hub.corpusOrigins[execSonarHint])
			}
			args := &SyncArgs{
				ID:            hub.id,
				Execs:         hub.stats.execs,
				Restarts:      hub.stats.restarts,
				CoverFullness: hub.corpusCoverSize,
			}
			hub.stats.execs = 0
			hub.stats.restarts = 0
			var res SyncRes
			if err := hub.coordinator.Call("Coordinator.Sync", args, &res); err != nil {
				log.Printf("sync call failed: %v, reconnection to coordinator", err)
				if err := hub.connect(); err != nil {
					log.Printf("failed to connect to coordinator: %v, killing worker", err)
					return
				}
			}
			if len(res.Inputs) > 0 {
				hub.triageQueue = append(hub.triageQueue, res.Inputs...)
			}
			if hub.corpusStale {
				hub.updateScores()
				hub.corpusStale = false
			}

		case triageC <- triageInput:
			// Send new input to workers for triage.
			if len(hub.triageQueue) > 0 {
				n := len(hub.triageQueue) - 1
				triageInput = hub.triageQueue[n]
				hub.triageQueue[n] = CoordinatorInput{}
				hub.triageQueue = hub.triageQueue[:n]
			} else {
				triageC = nil
				triageInput = CoordinatorInput{}
			}

		case s := <-hub.syncC:
			// Sync from a worker.
			hub.stats.execs += s.execs
			hub.stats.restarts += s.restarts

		case input := <-hub.newInputC:
			// New interesting input from workers.
			ro := hub.ro.Load().(*ROData)
			if !compareCover(ro.corpusCover, input.cover) {
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
			hub.updateMaxCover(input.cover)
			ro1.corpusCover = makeCopy(ro.corpusCover)
			hub.corpusCoverSize = updateMaxCover(ro1.corpusCover, input.cover)
			if input.res > 0 || input.typ == execBootstrap {
				ro1.verse = versifier.BuildVerse(ro.verse, input.data)
			}
			hub.ro.Store(ro1)
			hub.corpusOrigins[input.typ]++

			if input.mine {
				if err := hub.coordinator.Call("Coordinator.NewInput", NewInputArgs{hub.id, input.data, uint64(input.depth)}, nil); err != nil {
					log.Printf("new input call failed: %v, reconnecting to coordinator", err)
					if err := hub.connect(); err != nil {
						log.Printf("failed to connect to coordinator: %v, killing worker", err)
						return
					}
				}
			}

			if *flagDumpCover {
				dumpCover(filepath.Join(*flagWorkdir, "coverprofile"), ro.coverBlocks, ro.corpusCover)
			}

		case crash := <-hub.newCrasherC:
			// New crasher from workers. Woohoo!
			if crash.Hanging || !*flagDup {
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
				if !*flagDup {
					ro1.suppressions = make(map[Sig]struct{})
					for k, v := range ro.suppressions {
						ro1.suppressions[k] = v
					}
					ro1.suppressions[hash(crash.Suppression)] = struct{}{}
				}
				hub.ro.Store(ro1)
			}
			if err := hub.coordinator.Call("Coordinator.NewCrasher", crash, nil); err != nil {
				log.Printf("new crasher call failed: %v", err)
			}
		}
	}
}

// Preliminary cover update to prevent new input thundering herd.
// This function is synchronous to reduce latency.
func (hub *Hub) updateMaxCover(cover []byte) bool {
	oldMaxCover := hub.maxCover.Load().([]byte)
	if !compareCover(oldMaxCover, cover) {
		return false
	}
	hub.maxCoverMu.Lock()
	defer hub.maxCoverMu.Unlock()
	oldMaxCover = hub.maxCover.Load().([]byte)
	if !compareCover(oldMaxCover, cover) {
		return false
	}
	maxCover := makeCopy(oldMaxCover)
	updateMaxCover(maxCover, cover)
	hub.maxCover.Store(maxCover)
	return true
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

	// Phase 1: calculate score for each input independently.
	for i, inp := range corpus {
		score := defScore

		// Execution time multiplier 0.1-3x.
		// Fuzzing faster inputs increases efficiency.
		execTime := float64(inp.execTime) / float64(avgExecTime)
		if execTime > 10 {
			score /= 10
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

		// User boost (Fuzz function return value) multiplier 1-2x.
		// We don't know what it is, but user said so.
		if inp.res > 0 {
			// Assuming this is a correct input (e.g. deserialized successfully).
			score *= 2
		}

		if score < minScore {
			score = minScore
		} else if score > maxScore {
			score = maxScore
		}
		corpus[i].score = int(score)
	}

	// Phase 2: Choose a minimal set of (favored) inputs that give full coverage.
	// Non-favored inputs receive minimal score.
	type Candidate struct {
		index  int
		score  int
		chosen bool
	}
	candidates := make([]Candidate, CoverSize)
	for idx, inp := range corpus {
		corpus[idx].favored = false
		for i, c := range inp.cover {
			if c == 0 {
				continue
			}
			c = roundUpCover(c)
			if c != ro.corpusCover[i] {
				continue
			}
			if c > ro.corpusCover[i] {
				log.Fatalf("bad")
			}
			if candidates[i].score < inp.score {
				candidates[i].index = idx
				candidates[i].score = inp.score
			}
		}
	}
	for ci, cand := range candidates {
		if cand.score == 0 {
			continue
		}
		inp := &corpus[cand.index]
		inp.favored = true
		for i := ci + 1; i < CoverSize; i++ {
			c := inp.cover[i]
			if c == 0 {
				continue
			}
			c = roundUpCover(c)
			if c != ro.corpusCover[i] {
				continue
			}
			candidates[i].score = 0
		}
	}
	scoreSum := 0
	for i, inp := range corpus {
		if !inp.favored {
			inp.score = minScore
		}
		scoreSum += inp.score
		corpus[i].runningScoreSum = scoreSum
	}

	hub.ro.Store(ro1)
}
