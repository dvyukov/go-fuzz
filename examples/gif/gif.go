// Copyright 2015 Dmitry Vyukov. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

package gif

import (
	"bytes"
	"fmt"
	"image/gif"

	"github.com/dvyukov/go-fuzz/examples/fuzz"
)

func Fuzz(data []byte) int {
	cfg, err := gif.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return 0
	}
	if cfg.Width*cfg.Height > 1e6 {
		return 0
	}
	img, err := gif.Decode(bytes.NewReader(data))
	if err != nil {
		return 0
	}
	for c := 1; c <= 256; c += 21 {
		var w bytes.Buffer
		err = gif.Encode(&w, img, &gif.Options{NumColors: c})
		if err != nil {
			panic(err)
		}
		img1, err := gif.Decode(&w)
		if err != nil {
			panic(err)
		}
		b0 := img.Bounds()
		b1 := img1.Bounds()
		if b0.Max.X-b0.Min.X != b1.Max.X-b1.Min.X || b0.Max.Y-b0.Min.Y != b1.Max.Y-b1.Min.Y {
			fmt.Printf("img0: %#v\n", img.Bounds())
			fmt.Printf("img1: %#v\n", img1.Bounds())
			panic("bounds changed")
		}
	}
	return 1
}

func FuzzAll(data []byte) int {
	cfg, err := gif.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return 0
	}
	if cfg.Width*cfg.Height > 1e6 {
		return 0
	}
	img, err := gif.DecodeAll(bytes.NewReader(data))
	if err != nil {
		return 0
	}
	w := new(bytes.Buffer)
	err = gif.EncodeAll(w, img)
	if err != nil {
		panic(err)
	}
	img1, err := gif.DecodeAll(w)
	if err != nil {
		panic(err)
	}
	if img.LoopCount == 0 && img1.LoopCount == -1 {
		// https://github.com/golang/go/issues/11287
		img1.LoopCount = 0
	}
	// https://github.com/golang/go/issues/11288
	img1.Disposal = img.Disposal
	if !fuzz.DeepEqual(img, img1) {
		fmt.Printf("gif0: %#v\n", img)
		fmt.Printf("gif1: %#v\n", img1)
		panic("gif changed")
	}
	return 1
}
