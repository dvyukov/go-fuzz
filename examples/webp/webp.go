package webp

import (
	"bytes"
	"golang.org/x/image/webp"
)

func Fuzz(data []byte) int {
	if _, err := webp.Decode(bytes.NewReader(data)); err != nil {
		return 0
	}
	return 1
}
