// Copyright 2019 go-fuzz project authors. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

package main

import (
	"testing"

	. "github.com/dvyukov/go-fuzz/go-fuzz-defs"
)

func BenchmarkCompareCoverBody(b *testing.B) {
	base := make([]byte, CoverSize)
	cur := make([]byte, CoverSize)

	// Set 1 at both ends, so that it is easy regardless of which end compareCoverBody starts from.
	cur[0] = 1
	cur[CoverSize-1] = 1
	b.Run("easy", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if !compareCoverBody(base, cur) {
				b.Fatalf("cur should have increased coverage")
			}
		}
	})
	cur[0] = 0
	cur[CoverSize-1] = 0

	// Set 1 in the middle, so that it is hard regardless of which end compareCoverBody starts from.
	cur[CoverSize/2] = 1
	b.Run("hard", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			if !compareCoverBody(base, cur) {
				b.Fatalf("cur should have increased coverage")
			}
		}
	})
}
