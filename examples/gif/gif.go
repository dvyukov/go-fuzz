package gif

import (
	"bytes"
	"image/gif"
)

func Fuzz(data []byte) int {
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
