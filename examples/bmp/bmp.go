package bmp

import (
	"fmt"
	"bytes"
	"encoding/hex"
	"golang.org/x/image/bmp"
)

func Fuzz(data []byte) int {
	img, err := bmp.Decode(bytes.NewReader(data))
	if err != nil {
		return 0
	}
	var w bytes.Buffer
	err = bmp.Encode(&w, img)
	if err != nil {
		panic(err)
	}
	enc := w.Bytes()
	img1, err := bmp.Decode(&w)
	if err != nil {
		panic(err)
	}
	var w1 bytes.Buffer
	err = bmp.Encode(&w1, img1)
	if err != nil {
		panic(err)
	}
	enc1 := w1.Bytes()
	if !bytes.Equal(enc, enc1) {
		fmt.Printf("image1: [%v]%v\n", len(enc), hex.EncodeToString(enc))
		fmt.Printf("image2: [%v]%v\n", len(enc1), hex.EncodeToString(enc1))
		panic("not equal")
	}
	return 1
}
