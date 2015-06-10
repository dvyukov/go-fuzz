package png

import (
	"bytes"
	"fmt"
	"image/png"
	"reflect"
)

func Fuzz(data []byte) int {
	cfg, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return 0
	}
	if cfg.Width*cfg.Height > 1e6 {
		return 0
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return 0
	}
	for _, c := range []png.CompressionLevel{png.DefaultCompression, png.NoCompression, png.BestSpeed, png.BestCompression} {
		var w bytes.Buffer
		e := &png.Encoder{c}
		err = e.Encode(&w, img)
		if err != nil {
			panic(err)
		}
		img1, err := png.Decode(&w)
		if err != nil {
			panic(err)
		}
		if !reflect.DeepEqual(img.Bounds(), img1.Bounds()) {
			fmt.Printf("bounds0: %#v\n", img.Bounds())
			fmt.Printf("bounds1: %#v\n", img1.Bounds())
			panic("bounds have changed")
		}
	}
	return 1
}
