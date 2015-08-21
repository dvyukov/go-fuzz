// Copyright 2015 Dmitry Vyukov. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

// +build !amd64

package main

func compareCoverBody(base, cur []byte) bool {
	return compareCoverDump(base, cur)
}
