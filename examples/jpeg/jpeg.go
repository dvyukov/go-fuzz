package jpeg

import (
	"bytes"
	"image/jpeg"
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
	var w bytes.Buffer
	err = jpeg.Encode(&w, img, nil)
	if err != nil {
		panic(err)
	}
	return 1
}
