package gif

import (
	"bytes"
	"fmt"
	"image/gif"
	"reflect"
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
		if !reflect.DeepEqual(img.Bounds(), img1.Bounds()) {
			fmt.Printf("img0: %#v\n", img.Bounds())
			fmt.Printf("img1: %#v\n", img1.Bounds())
			panic("bounds changed")
		}
	}
	return 1
}
