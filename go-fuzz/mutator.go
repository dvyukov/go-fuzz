package main

import (
	"math/rand"
	"time"
)

type Mutator struct {
	r *rand.Rand
}

func newMutator() *Mutator {
	return &Mutator{r: rand.New(rand.NewSource(time.Now().UnixNano()))}
}

func (m *Mutator) rand(n int) int {
	return m.r.Intn(n)
}

func (m *Mutator) generate(corpus []Input) ([]byte, int) {
	if len(corpus) == 0 {
		return []byte{byte(m.rand(256))}, 0
	}
	idx := m.rand(len(corpus))
	return m.mutate(corpus[idx].data, corpus)
}

func (m *Mutator) mutate(data []byte, corpus []Input) ([]byte, int) {
	res := make([]byte, len(data))
	copy(res, data)
	for i := m.rand(5); i >= 0; i-- {
		switch m.rand(5) {
		case 0:
			if len(res) > 0 {
				pos := m.rand(len(res))
				copy(res[pos:], res[pos+1:])
				res = res[:len(res)-1]
			}
		case 1:
			if len(res) < 100 {
				if len(res) == 0 {
					res = append(res, byte(m.rand(256)))
				} else {
					pos := m.rand(len(res))
					res = append(res, 0)
					copy(res[pos+1:], res[pos:])
					res[pos] = byte(m.rand(256))
				}
			}
		case 2:
			if len(res) > 0 {
				pos := m.rand(len(res))
				res[pos] ^= 1 << uint(m.rand(8))
			}
		case 3:
			if len(res) > 32 {
				pos0 := m.rand(len(res) - 1)
				pos1 := pos0 + m.rand(len(res)-pos0)
				copy(res[pos0:], res[pos1:])
				res = res[:len(res)-(pos1-pos0)]
			}
		case 4:
			// TODO: do less frequently
			idx := m.rand(len(corpus))
			res = m.crossover(res, corpus[idx].data)
		}
	}
	// TODO: calculate depth
	return res, 0
}

func (m *Mutator) crossover(data0, data1 []byte) []byte {
	// TODO: don't copy
	res := make([]byte, 0, len(data0)+len(data1))
	copy(res, data0)
	for i := m.rand(3); i >= 0; i-- {
		if len(data0) > 0 {
			pos0 := m.rand(len(data0))
			res = append(res, data0[:pos0]...)
			data0 = data0[pos0:]
		}
		if len(data1) > 0 {
			pos1 := m.rand(len(data1))
			res = append(res, data1[:pos1]...)
			data1 = data1[pos1:]
		}
	}
	res = append(res, data0...)
	return res
}
