package webp

import (
	"bytes"
	"golang.org/x/image/webp"
)

func Fuzz(data []byte) int {
	cfg, err := webp.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return 0
	}
	if cfg.Width*cfg.Height > 1e6 {
		return 0
	}
	if _, err := webp.Decode(bytes.NewReader(data)); err != nil {
		return 0
	}
	return 1
}
