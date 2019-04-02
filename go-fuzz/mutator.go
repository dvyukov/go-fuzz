// Copyright 2015 go-fuzz project authors. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

package main

import (
	"encoding/binary"
	"sort"
	"strconv"

	. "github.com/dvyukov/go-fuzz/go-fuzz-defs"
	"github.com/dvyukov/go-fuzz/go-fuzz/internal/pcg"
)

type Mutator struct {
	r *pcg.Rand
}

func newMutator() *Mutator {
	return &Mutator{r: pcg.New()}
}

func (m *Mutator) rand(n int) int {
	return m.r.Intn(n)
}

// randbig generates a number in [0, 2³⁰).
func (m *Mutator) randbig() int64 {
	return int64(m.r.Uint32() >> 2)
}

func (m *Mutator) randByteOrder() binary.ByteOrder {
	if m.r.Bool() {
		return binary.LittleEndian
	}
	return binary.BigEndian
}

func (m *Mutator) generate(ro *ROData) ([]byte, int) {
	corpus := ro.corpus
	scoreSum := corpus[len(corpus)-1].runningScoreSum
	weightedIdx := m.rand(scoreSum)
	idx := sort.Search(len(corpus), func(i int) bool {
		return corpus[i].runningScoreSum > weightedIdx
	})
	input := &corpus[idx]
	return m.mutate(input.data, ro), input.depth + 1
}

func (m *Mutator) mutate(data []byte, ro *ROData) []byte {
	corpus := ro.corpus
	res := make([]byte, len(data))
	copy(res, data)
	nm := 1 + m.r.Exp2()
	for iter := 0; iter < nm; iter++ {
		switch m.rand(20) {
		case 0:
			// Remove a range of bytes.
			if len(res) <= 1 {
				iter--
				continue
			}
			pos0 := m.rand(len(res))
			pos1 := pos0 + m.chooseLen(len(res)-pos0)
			copy(res[pos0:], res[pos1:])
			res = res[:len(res)-(pos1-pos0)]
		case 1:
			// Insert a range of random bytes.
			pos := m.rand(len(res) + 1)
			n := m.chooseLen(10)
			for i := 0; i < n; i++ {
				res = append(res, 0)
			}
			copy(res[pos+n:], res[pos:])
			for i := 0; i < n; i++ {
				res[pos+i] = byte(m.rand(256))
			}
		case 2:
			// Duplicate a range of bytes.
			if len(res) <= 1 {
				iter--
				continue
			}
			src := m.rand(len(res))
			dst := m.rand(len(res))
			for dst == src {
				dst = m.rand(len(res))
			}
			n := m.chooseLen(len(res) - src)
			tmp := make([]byte, n)
			copy(tmp, res[src:])
			for i := 0; i < n; i++ {
				res = append(res, 0)
			}
			copy(res[dst+n:], res[dst:])
			for i := 0; i < n; i++ {
				res[dst+i] = tmp[i]
			}
		case 3:
			// Copy a range of bytes.
			if len(res) <= 1 {
				iter--
				continue
			}
			src := m.rand(len(res))
			dst := m.rand(len(res))
			for dst == src {
				dst = m.rand(len(res))
			}
			n := m.chooseLen(len(res) - src)
			if dst > len(res) || src+n > len(res) {
				println(len(res), dst, src, n)
			}
			copy(res[dst:], res[src:src+n])
		case 4:
			// Bit flip. Spooky!
			if len(res) == 0 {
				iter--
				continue
			}
			pos := m.rand(len(res))
			res[pos] ^= 1 << uint(m.rand(8))
		case 5:
			// Set a byte to a random value.
			if len(res) == 0 {
				iter--
				continue
			}
			pos := m.rand(len(res))
			res[pos] ^= byte(m.rand(255)) + 1
		case 6:
			// Swap 2 bytes.
			if len(res) <= 1 {
				iter--
				continue
			}
			src := m.rand(len(res))
			dst := m.rand(len(res))
			for dst == src {
				dst = m.rand(len(res))
			}
			res[src], res[dst] = res[dst], res[src]
		case 7:
			// Add/subtract from a byte.
			if len(res) == 0 {
				iter--
				continue
			}
			pos := m.rand(len(res))
			v := byte(m.rand(35) + 1)
			if m.r.Bool() {
				res[pos] += v
			} else {
				res[pos] -= v
			}
		case 8:
			// Add/subtract from a uint16.
			if len(res) < 2 {
				iter--
				continue
			}
			pos := m.rand(len(res) - 1)
			buf := res[pos:]
			v := uint16(m.rand(35) + 1)
			if m.r.Bool() {
				v = 0 - v
			}
			enc := m.randByteOrder()
			enc.PutUint16(buf, enc.Uint16(buf)+v)
		case 9:
			// Add/subtract from a uint32.
			if len(res) < 4 {
				iter--
				continue
			}
			pos := m.rand(len(res) - 3)
			buf := res[pos:]
			v := uint32(m.rand(35) + 1)
			if m.r.Bool() {
				v = 0 - v
			}
			enc := m.randByteOrder()
			enc.PutUint32(buf, enc.Uint32(buf)+v)
		case 10:
			// Add/subtract from a uint64.
			if len(res) < 8 {
				iter--
				continue
			}
			pos := m.rand(len(res) - 7)
			buf := res[pos:]
			v := uint64(m.rand(35) + 1)
			if m.r.Bool() {
				v = 0 - v
			}
			enc := m.randByteOrder()
			enc.PutUint64(buf, enc.Uint64(buf)+v)
		case 11:
			// Replace a byte with an interesting value.
			if len(res) == 0 {
				iter--
				continue
			}
			pos := m.rand(len(res))
			res[pos] = byte(interesting8[m.rand(len(interesting8))])
		case 12:
			// Replace an uint16 with an interesting value.
			if len(res) < 2 {
				iter--
				continue
			}
			pos := m.rand(len(res) - 1)
			buf := res[pos:]
			v := uint16(interesting16[m.rand(len(interesting16))])
			m.randByteOrder().PutUint16(buf, v)
		case 13:
			// Replace an uint32 with an interesting value.
			if len(res) < 4 {
				iter--
				continue
			}
			pos := m.rand(len(res) - 3)
			buf := res[pos:]
			v := uint32(interesting32[m.rand(len(interesting32))])
			m.randByteOrder().PutUint32(buf, v)
		case 14:
			// Replace an ascii digit with another digit.
			var digits []int
			for i, v := range res {
				if v >= '0' && v <= '9' {
					digits = append(digits, i)
				}
			}
			if len(digits) == 0 {
				iter--
				continue
			}
			pos := m.rand(len(digits))
			was := res[digits[pos]]
			now := was
			for was == now {
				now = byte(m.rand(10)) + '0'
			}
			res[digits[pos]] = now
		case 15:
			// Replace a multi-byte ASCII number with another number.
			type arange struct {
				start int
				end   int
			}
			var numbers []arange
			start := -1
			for i, v := range res {
				if (v >= '0' && v <= '9') || (start == -1 && v == '-') {
					if start == -1 {
						start = i
					} else if i == len(res)-1 {
						// At final byte of input.
						if i-start > 0 {
							numbers = append(numbers, arange{start, i + 1})
						}
					}
				} else {
					if start != -1 && i-start > 1 {
						numbers = append(numbers, arange{start, i})
						start = -1
					}
				}
			}
			if len(numbers) == 0 {
				iter--
				continue
			}
			var v int64
			switch m.rand(4) {
			case 0:
				v = int64(m.rand(1000))
			case 1:
				v = m.randbig()
			case 2:
				v = m.randbig() * m.randbig()
			case 3:
				v = -m.randbig()
			}
			r := numbers[m.rand(len(numbers))]
			if res[r.start] == '-' {
				// If we started with a negative number, invert the sign of v.
				// The idea here is that negative numbers will mostly stay negative;
				// we only generate a negative (positive) replacement 1/4th of the time.
				v *= -1
			}
			str := strconv.FormatInt(v, 10)
			tmp := make([]byte, len(res)-(r.end-r.start)+len(str))
			copy(tmp, res[:r.start])
			copy(tmp[r.start:], str)
			copy(tmp[r.start+len(str):], res[r.end:])
			res = tmp
		case 16:
			// Splice another input.
			if len(res) < 4 || len(corpus) < 2 {
				iter--
				continue
			}
			other := corpus[m.rand(len(corpus))].data
			if len(other) < 4 || &res[0] == &other[0] {
				iter--
				continue
			}
			// Find common prefix and suffix.
			idx0 := 0
			for idx0 < len(res) && idx0 < len(other) && res[idx0] == other[idx0] {
				idx0++
			}
			idx1 := 0
			for idx1 < len(res) && idx1 < len(other) && res[len(res)-idx1-1] == other[len(other)-idx1-1] {
				idx1++
			}
			// If diffing parts are too small, there is no sense in splicing, rely on byte flipping.
			diff := min(len(res)-idx0-idx1, len(other)-idx0-idx1)
			if diff < 4 {
				iter--
				continue
			}
			copy(res[idx0:idx0+m.rand(diff-2)+1], other[idx0:])
		case 17:
			// Insert a part of another input.
			if len(res) < 4 || len(corpus) < 2 {
				iter--
				continue
			}
			other := corpus[m.rand(len(corpus))].data
			if len(other) < 4 || &res[0] == &other[0] {
				iter--
				continue
			}
			pos0 := m.rand(len(res) + 1)
			pos1 := m.rand(len(other) - 2)
			n := m.chooseLen(len(other)-pos1-2) + 2
			for i := 0; i < n; i++ {
				res = append(res, 0)
			}
			copy(res[pos0+n:], res[pos0:])
			for i := 0; i < n; i++ {
				res[pos0+i] = other[pos1+i]
			}
		case 18:
			// Insert a literal.
			// TODO: encode int literals in big-endian, base-128, etc.
			if len(ro.intLits) == 0 && len(ro.strLits) == 0 {
				iter--
				continue
			}
			var lit []byte
			if len(ro.strLits) != 0 && m.r.Bool() {
				lit = []byte(ro.strLits[m.rand(len(ro.strLits))])
			} else {
				lit = ro.intLits[m.rand(len(ro.intLits))]
				if m.rand(3) == 0 {
					lit = reverse(lit)
				}
			}
			pos := m.rand(len(res) + 1)
			for i := 0; i < len(lit); i++ {
				res = append(res, 0)
			}
			copy(res[pos+len(lit):], res[pos:])
			copy(res[pos:], lit)
		case 19:
			// Replace with literal.
			if len(ro.intLits) == 0 && len(ro.strLits) == 0 {
				iter--
				continue
			}
			var lit []byte
			if len(ro.strLits) != 0 && m.r.Bool() {
				lit = []byte(ro.strLits[m.rand(len(ro.strLits))])
			} else {
				lit = ro.intLits[m.rand(len(ro.intLits))]
				if m.rand(3) == 0 {
					lit = reverse(lit)
				}
			}
			if len(lit) >= len(res) {
				iter--
				continue
			}
			pos := m.rand(len(res) - len(lit))
			copy(res[pos:], lit)
		}
	}
	if len(res) > MaxInputSize {
		res = res[:MaxInputSize]
	}
	return res
}

// chooseLen chooses length of range mutation.
// It gives preference to shorter ranges.
func (m *Mutator) chooseLen(n int) int {
	switch x := m.rand(100); {
	case x < 90:
		return m.rand(min(8, n)) + 1
	case x < 99:
		return m.rand(min(32, n)) + 1
	default:
		return m.rand(n) + 1
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

var (
	interesting8  = []int8{-128, -1, 0, 1, 16, 32, 64, 100, 127}
	interesting16 = []int16{-32768, -129, 128, 255, 256, 512, 1000, 1024, 4096, 32767}
	interesting32 = []int32{-2147483648, -100663046, -32769, 32768, 65535, 65536, 100663045, 2147483647}
)

func init() {
	for _, v := range interesting8 {
		interesting16 = append(interesting16, int16(v))
	}
	for _, v := range interesting16 {
		interesting32 = append(interesting32, int32(v))
	}
}
