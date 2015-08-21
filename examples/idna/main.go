// Copyright 2015 Dmitry Vyukov. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

package idna

import (
	"fmt"

	"golang.org/x/net/idna"
)

func Fuzz(data []byte) int {
	for _, v := range data {
		if v <= 0x20 || v >= 0x80 {
			return 0
		}
	}
	dec, err := idna.ToUnicode(string(data))
	if err != nil {
		return 0
	}
	enc, err := idna.ToASCII(dec)
	if err != nil {
		fmt.Printf("data: %q\n", data)
		fmt.Printf("dec : %q\n", dec)
		panic(err)
	}
	dec1, err := idna.ToUnicode(enc)
	if err != nil {
		fmt.Printf("data: %q\n", data)
		fmt.Printf("dec : %q\n", dec)
		fmt.Printf("enc : %q\n", enc)
		panic(err)
	}
	if dec != dec1 {
		fmt.Printf("data: %q\n", data)
		fmt.Printf("dec : %q\n", dec)
		fmt.Printf("enc : %q\n", enc)
		fmt.Printf("dec1: %q\n", dec1)
		panic("not equal")
	}
	return 1
}
