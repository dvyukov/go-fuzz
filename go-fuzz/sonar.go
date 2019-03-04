// Copyright 2015 go-fuzz project authors. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

package main

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"log"
	"path/filepath"
	"strconv"
	"sync"
	"unicode"

	. "github.com/dvyukov/go-fuzz/go-fuzz-defs"
)

type SonarSite struct {
	id  int    // unique site id (const)
	loc string // file:line.pos,line.pos (const)
	sync.Mutex
	dynamic    bool   // both operands are not constant
	takenFuzz  [2]int // number of times condition evaluated to false/true during fuzzing
	takenTotal [2]int // number of times condition evaluated to false/true in total
	val        [2][]byte
}

type SonarSample struct {
	site  *SonarSite
	flags byte
	val   [2][]byte
}

func (w *Worker) parseSonarData(sonar []byte) (res []SonarSample) {
	ro := w.hub.ro.Load().(*ROData)
	sonar = makeCopy(sonar)
	for len(sonar) > SonarHdrLen {
		id := binary.LittleEndian.Uint32(sonar)
		flags := byte(id)
		id >>= 8
		n1 := sonar[4]
		n2 := sonar[5]
		sonar = sonar[SonarHdrLen:]
		if n1 > SonarMaxLen || n2 > SonarMaxLen || len(sonar) < int(n1)+int(n2) {
			log.Fatalf("corrupted sonar data: hdr=[%v/%v/%v] data=%v", flags, n1, n2, len(sonar))
		}
		v1 := makeCopy(sonar[:n1])
		v2 := makeCopy(sonar[n1 : n1+n2])
		sonar = sonar[n1+n2:]

		// Trim trailing 0x00 and 0xff bytes (we don't know exact size of operands).
		if flags&SonarString == 0 {
			for len(v1) > 0 || len(v2) > 0 {
				i := len(v1) - 1
				if len(v2) > len(v1) {
					i = len(v2) - 1
				}
				var c1, c2 byte
				if i < len(v1) {
					c1 = v1[i]
				}
				if i < len(v2) {
					c2 = v2[i]
				}
				if (c1 == 0 || c1 == 0xff) && (c2 == 0 || c2 == 0xff) {
					if i < len(v1) {
						v1 = v1[:i]
					}
					if i < len(v2) {
						v2 = v2[:i]
					}
					continue
				}
				break
			}
		}

		res = append(res, SonarSample{&ro.sonarSites[id], flags, [2][]byte{v1, v2}})
	}
	return res
}

func (w *Worker) processSonarData(data, sonar []byte, depth int, smash bool) {
	ro := w.hub.ro.Load().(*ROData)
	updated := false
	checked := make(map[string]struct{})
	samples := w.parseSonarData(sonar)
	for _, sam := range samples {
		// TODO: extract literal corpus from sonar instead of from source.
		// This should give smaller, better corpus which does not contain literals from dead code.

		// TODO: detect loop counters (small incrementing/decrementing values on the same site).
		// Either ignore them or handle differently (e.g. alter a string length).

		site := sam.site
		flags := sam.flags
		v1 := sam.val[0]
		v2 := sam.val[1]
		res := sam.evaluate()
		// Ignore sites that has at least one const operand and
		// are already taken both ways enough times.
		upd, skip := site.update(sam, smash, res)
		if upd {
			updated = true
		}
		if skip {
			continue
		}
		if smash && bytes.Equal(v1, v2) {
			// We systematically mutate all bytes during smashing,
			// no point in trying to break equality here.
			continue
		}
		testInput := func(tmp []byte) {
			w.testInput(tmp, depth+1, execSonarHint)
		}
		check := func(indexdata, v1, v2 []byte) {
			if len(v1) == 0 || bytes.Equal(v1, v2) || !bytes.Contains(indexdata, v1) {
				return
			}
			if len(indexdata) != len(data) {
				// We use indices into indexdata to construct indices into data below.
				// If they don't have the same size, this operation is nonsensical and may panic.
				panic("len(indexdata) != len(data)")
			}
			vv := string(v1) + "\t|\t" + string(v2)
			if _, ok := checked[vv]; ok {
				return
			}
			checked[vv] = struct{}{}
			pos := 0
			for {
				i := bytes.Index(indexdata[pos:], v1)
				if i == -1 {
					break
				}
				i += pos
				pos = i + 1
				tmp := make([]byte, len(data)-len(v1)+len(v2))
				copy(tmp, data[:i])
				copy(tmp[i:], v2)
				copy(tmp[i+len(v2):], data[i+len(v1):])
				if len(tmp) > CoverSize {
					tmp = tmp[:CoverSize]
				}
				testInput(tmp)
				if flags&SonarString != 0 && len(v1) != len(v2) && len(tmp) < CoverSize {
					// Update length field.
					// TODO: handle multi-byte/big-endian/base-128 length fields.
					diff := byte(len(v2) - len(v1))
					for idx := i - 1; idx >= 0 && idx+5 >= i; idx-- {
						tmp[idx] += diff
						testInput(tmp)
						tmp[idx] -= diff
					}
				}
			}
		}
		check1 := func(v1, v2 []byte) {
			check(data, v1, v2)
			// TODO: for strings check upper/lower case.
			if flags&SonarString != 0 {
				if bytes.Equal(v1, bytes.ToLower(v1)) && bytes.Equal(v2, bytes.ToLower(v2)) {
					if lower := bytes.ToLower(data); len(lower) == len(data) {
						check(lower, v1, v2)
					}
				}
				if bytes.Equal(v1, bytes.ToUpper(v1)) && bytes.Equal(v2, bytes.ToUpper(v2)) {
					if upper := bytes.ToUpper(data); len(upper) == len(data) {
						check(upper, v1, v2)
					}
				}
			} else {
				// Try several common wire encodings of the values:
				// network format (big endian), hex, base-128.
				// TODO: try more encodings if it proves to be useful:
				// base-64, quoted-printable, xml-escaping, hex+increment/decrement.

				if len(v1) == 1 && len(v2) == 1 && unicode.IsLower(rune(v1[0])) && unicode.IsLower(rune(v2[0])) {
					if lower := bytes.ToLower(data); len(lower) == len(data) {
						check(lower, v1, v2)
					}
				}
				if len(v1) == 1 && len(v2) == 1 && unicode.IsUpper(rune(v1[0])) && unicode.IsUpper(rune(v2[0])) {
					if upper := bytes.ToUpper(data); len(upper) == len(data) {
						check(upper, v1, v2)
					}
				}

				// Increment and decrement take care of less and greater comparison operators
				// as well as of off-by-one bugs.
				check(data, v1, increment(v2))
				check(data, v1, decrement(v2))

				// Also try big-endian increments/decrements.
				if len(v1) > 1 {
					check(data, reverse(v1), reverse(v2))
					check(data, reverse(v1), reverse(increment(v2)))
					check(data, reverse(v1), reverse(decrement(v2)))
					check(data, v1, reverse(increment(reverse(v2))))
					check(data, v1, reverse(decrement(reverse(v2))))
				}

				// Base-128.
				// TODO: try to treat the value as negative.
				var u1, u2 uint64
				for i := 0; i < len(v1); i++ {
					u1 += uint64(v1[i]) << uint(i*8)
				}
				for i := 0; i < len(v2); i++ {
					u2 += uint64(v2[i]) << uint(i*8)
				}
				if u1 > 127 || u2 > 127 { // otherwise it's the same as byte replacement
					var vv1, vv2 [10]byte
					n1 := binary.PutUvarint(vv1[:], u1)
					n2 := binary.PutUvarint(vv2[:], u2)
					check(data, vv1[:n1], vv2[:n2])

					// Increment/decrement in base-128.
					n1 = binary.PutUvarint(vv1[:], u1+1)
					n2 = binary.PutUvarint(vv2[:], u2+1)
					check(data, vv1[:n1], vv2[:n2])
					n1 = binary.PutUvarint(vv1[:], u1-1)
					n2 = binary.PutUvarint(vv2[:], u2-1)
					check(data, vv1[:n1], vv2[:n2])
				}

				// Ascii-encoding.
				// TODO: try to treat the value as negative.
				s1 := strconv.FormatUint(u1, 10)
				s2 := strconv.FormatUint(u2, 10)
				check(data, []byte(s1), []byte(s2))
			}
			check(data, []byte(hex.EncodeToString(v1)), []byte(hex.EncodeToString(v2)))
		}
		if flags&SonarConst1 == 0 {
			check1(v1, v2)
		}
		if flags&SonarConst2 == 0 {
			check1(v2, v1)
		}
	}
	if updated && *flagDumpCover {
		dumpMu.Lock()
		defer dumpMu.Unlock()
		dumpSonar(filepath.Join(*flagWorkdir, "sonarprofile"), ro.sonarSites)
	}
}

var dumpMu sync.Mutex

func (site *SonarSite) update(sam SonarSample, smash, resb bool) (updated, skip bool) {
	res := 0
	if resb {
		res = 1
	}
	site.Lock()
	defer site.Unlock()
	v1 := sam.val[0]
	v2 := sam.val[1]
	if !site.dynamic && sam.flags&SonarConst1+sam.flags&SonarConst2 == 0 {
		if site.val[0] == nil {
			site.val[0] = makeCopy(v1)
		}
		if site.val[1] == nil {
			site.val[1] = makeCopy(v2)
		}
		if !bytes.Equal(site.val[0], v1) && !bytes.Equal(site.val[1], v2) {
			site.val[0] = nil
			site.val[1] = nil
			site.dynamic = true
		}
	}
	if site.takenTotal[res] == 0 {
		updated = true
	}
	site.takenTotal[res]++
	if !smash {
		site.takenFuzz[res]++
	}
	if !site.dynamic && !smash {
		// Skip this site if it has at least one const operand
		// and is taken both ways enough times.
		// Check sites that don't have const operands always,
		// because it can be a CRC-like verification, which
		// won't be cracked otherwise.
		if site.takenFuzz[0] > 10 && site.takenFuzz[1] > 10 || site.takenFuzz[0]+site.takenFuzz[1] > 100 {
			skip = true
			return
		}
	}
	return
}

func (sam *SonarSample) evaluate() bool {
	v1 := sam.val[0]
	v2 := sam.val[1]
	if sam.flags&SonarString != 0 {
		s1 := string(v1)
		s2 := string(v2)
		switch sam.flags & SonarOpMask {
		case SonarEQL:
			return s1 == s2
		case SonarNEQ:
			return s1 != s2
		case SonarLSS:
			return s1 < s2
		case SonarGTR:
			return s1 > s2
		case SonarLEQ:
			return s1 <= s2
		case SonarGEQ:
			return s1 >= s2
		default:
			panic("bad")
		}
	}
	if len(v1) == 0 || len(v2) == 0 || len(v1) > 8 || len(v2) > 8 || len(v1) != len(v2) {
		return false
	}
	v1 = makeCopy(v1)
	for len(v1) < 8 {
		if int8(v1[len(v1)-1]) >= 0 {
			v1 = append(v1, 0)
		} else {
			v1 = append(v1, 0xff)
		}
	}
	v2 = makeCopy(v2)
	for len(v2) < 8 {
		if int8(v2[len(v2)-1]) >= 0 {
			v2 = append(v2, 0)
		} else {
			v2 = append(v2, 0xff)
		}
	}
	// Note: assuming le machine.
	if sam.flags&SonarSigned == 0 {
		s1 := binary.LittleEndian.Uint64(v1)
		s2 := binary.LittleEndian.Uint64(v2)
		switch sam.flags & SonarOpMask {
		case SonarEQL:
			return s1 == s2
		case SonarNEQ:
			return s1 != s2
		case SonarLSS:
			return s1 < s2
		case SonarGTR:
			return s1 > s2
		case SonarLEQ:
			return s1 <= s2
		case SonarGEQ:
			return s1 >= s2
		default:
			panic("bad")
		}
	} else {
		s1 := int64(binary.LittleEndian.Uint64(v1))
		s2 := int64(binary.LittleEndian.Uint64(v2))
		switch sam.flags & SonarOpMask {
		case SonarEQL:
			return s1 == s2
		case SonarNEQ:
			return s1 != s2
		case SonarLSS:
			return s1 < s2
		case SonarGTR:
			return s1 > s2
		case SonarLEQ:
			return s1 <= s2
		case SonarGEQ:
			return s1 >= s2
		default:
			panic("bad")
		}
	}
}

func dumpSonarData(site *SonarSite, flags byte, v1, v2 []byte) {
	// Debug output.
	op := ""
	switch flags & SonarOpMask {
	case SonarEQL:
		op = "=="
	case SonarNEQ:
		op = "!="
	case SonarLSS:
		op = "<"
	case SonarGTR:
		op = ">"
	case SonarLEQ:
		op = "<="
	case SonarGEQ:
		op = ">="
	default:
		log.Fatalf("bad")
	}
	sign := ""
	if flags&SonarSigned != 0 {
		sign = "(signed)"
	}
	isstr := ""
	if flags&SonarString != 0 {
		isstr = "(string)"
	}
	const1 := ""
	if flags&SonarConst1 != 0 {
		const1 = "c"
	}
	const2 := ""
	if flags&SonarConst2 != 0 {
		const2 = "c"
	}
	log.Printf("SONAR %v%v %v %v%v %v%v %v",
		hex.EncodeToString(v1), const1, op, hex.EncodeToString(v2), const2, sign, isstr, site.loc)
}
