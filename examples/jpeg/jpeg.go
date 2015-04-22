package jpeg

import (
	"bytes"
	"image/jpeg"
)

func Fuzz(data []byte) int {
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
