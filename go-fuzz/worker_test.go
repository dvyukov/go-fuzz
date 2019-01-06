package main

import (
	"encoding/binary"
	"math"
	"testing"
)

func TestIncrementDecrement(t *testing.T) {
	b := make([]byte, 2)
	for i := 0; i < math.MaxUint16; i++ {
		u := uint16(i)
		binary.LittleEndian.PutUint16(b, u)

		b1 := increment(b)
		u1 := binary.LittleEndian.Uint16(b1)
		if u+1 != u1 {
			t.Fatalf("increment(%d) = %d, want %d", u, u1, u+1)
		}

		b1 = decrement(b)
		u1 = binary.LittleEndian.Uint16(b1)
		if u-1 != u1 {
			t.Fatalf("decrement(%d) = %d, want %d", u, u1, u-1)
		}
	}
}
