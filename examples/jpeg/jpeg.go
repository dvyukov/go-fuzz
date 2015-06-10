package jpeg

import (
	"bytes"
	"image/jpeg"
	"reflect"
)

func Fuzz(data []byte) int {
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return 0
	}
	if cfg.Width*cfg.Height > 1e6 {
		return 0
	}
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return 0
	}
	for q := 0; q <= 100; q += 10 {
		var w bytes.Buffer
		err = jpeg.Encode(&w, img, &jpeg.Options{q})
		if err != nil {
			panic(err)
		}
		img1, err := jpeg.Decode(&w)
		if err != nil {
			panic(err)
		}
		if !reflect.DeepEqual(img.Bounds(), img1.Bounds()) {
			panic("bounds changed")
		}
	}
	return 1
}
