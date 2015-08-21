// Copyright 2015 Dmitry Vyukov. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

package main

func compareCoverBody(base, cur []byte) bool {
	return compareCoverBody1(&base[0], &cur[0])
}

func compareCoverBody1(base, cur *byte) bool // in compare_amd64.s
