package gif

import (
	"bytes"
	"image/gif"
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
	var w bytes.Buffer
	err = gif.Encode(&w, img, nil)
	if err != nil {
		panic(err)
	}
	return 1
}
