package png

import (
	"bytes"
	"image/png"
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
	var w bytes.Buffer
	err = png.Encode(&w, img)
	if err != nil {
		panic(err)
	}
	return 1
}
