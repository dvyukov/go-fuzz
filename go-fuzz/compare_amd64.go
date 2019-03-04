// Copyright 2015 go-fuzz project authors. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

package main

func compareCoverBody(base, cur []byte) bool {
	if hasAVX2 {
		return compareCoverBodyAVX2(&base[0], &cur[0])
	}
	return compareCoverBodySSE2(&base[0], &cur[0])
}

func compareCoverBodySSE2(base, cur *byte) bool // in compare_amd64.s
func compareCoverBodyAVX2(base, cur *byte) bool // in compare_amd64.s
