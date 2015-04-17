package jpeg

import (
	"image/jpeg"
	"bytes"
)

func Fuzz(data []byte) {
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return
	}
	var w bytes.Buffer
	err = jpeg.Encode(&w, img, nil)
	if err != nil {
		panic(err)
	}
}
