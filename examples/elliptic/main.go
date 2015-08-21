// Copyright 2015 Dmitry Vyukov. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

package elliptic

import (
	"crypto/elliptic"
)

func Fuzz(data []byte) int {
	curves := []elliptic.Curve{
		elliptic.P224(),
		elliptic.P256(),
		elliptic.P384(),
		elliptic.P521(),
	}
	score := 0
	for _, c := range curves {
		x, y := elliptic.Unmarshal(c, data)
		if x != nil {
			score++
			elliptic.Marshal(c, x, y)
		}
	}
	return score
}
