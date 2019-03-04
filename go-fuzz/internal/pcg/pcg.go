// Copyright 2019 go-fuzz project authors. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

// Package pcg implements a 32 bit PRNG with a 64 bit period: pcg xsh rr 64 32.
// See https://www.pcg-random.org/ for more information.
// This implementation is geared specifically towards go-fuzz's needs:
// Simple creation and use, no reproducibility, no concurrency safety,
// just the methods go-fuzz needs, optimized for speed.
package pcg

import (
	"math/bits"
	"sync/atomic"
	"time"
)

var globalInc uint64 // PCG stream

const multiplier uint64 = 6364136223846793005

// Rand is a PRNG.
// It should not be copied or shared.
// No Rand methods are concurrency safe.
// They are small, and cheap to create.
// If in doubt: Just make another one.
type Rand struct {
	noCopy noCopy // help avoid mistakes: ask vet to ensure that we don't make a copy
	state  uint64
	inc    uint64
}

// New generates a new, seeded Rand, ready for use.
func New() *Rand {
	r := new(Rand)
	now := uint64(time.Now().UnixNano())
	inc := atomic.AddUint64(&globalInc, 1)
	r.state = now
	r.inc = (inc << 1) | 1
	r.step()
	r.state += now
	r.step()
	return r
}

func (r *Rand) step() {
	r.state *= multiplier
	r.state += r.inc
}

// Uint32 returns a pseudo-random uint32.
func (r *Rand) Uint32() uint32 {
	x := r.state
	r.step()
	return bits.RotateLeft32(uint32(((x>>18)^x)>>27), -int(x>>59))
}

// Intn returns a pseudo-random number in [0, n).
// n must fit in a uint32.
func (r *Rand) Intn(n int) int {
	if int(uint32(n)) != n {
		panic("large Intn")
	}
	return int(r.Uint32n(uint32(n)))
}

// Uint32n returns a pseudo-random number in [0, n).
//
// For implementation details, see:
// https://lemire.me/blog/2016/06/27/a-fast-alternative-to-the-modulo-reduction
// https://lemire.me/blog/2016/06/30/fast-random-shuffling
func (r *Rand) Uint32n(n uint32) uint32 {
	v := r.Uint32()
	prod := uint64(v) * uint64(n)
	low := uint32(prod)
	if low < n {
		thresh := uint32(-int32(n)) % n
		for low < thresh {
			v = r.Uint32()
			prod = uint64(v) * uint64(n)
			low = uint32(prod)
		}
	}
	return uint32(prod >> 32)
}

// Exp2 generates n with probability 1/2^(n+1).
func (r *Rand) Exp2() int {
	return bits.TrailingZeros32(r.Uint32())
}

// Bool generates a random bool.
func (r *Rand) Bool() bool {
	return r.Uint32()&1 == 0
}

// noCopy may be embedded into structs which must not be copied
// after the first use.
//
// See https://golang.org/issues/8005#issuecomment-190753527
// for details.
type noCopy struct{}

// Lock is a no-op used by -copylocks checker from `go vet`.
func (*noCopy) Lock()   {}
func (*noCopy) Unlock() {}
