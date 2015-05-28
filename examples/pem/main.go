package pem

import (
	"bytes"
	"encoding/pem"
)

func Fuzz(data []byte) int {
	b, _ := pem.Decode(data)
	if b == nil {
		return 0
	}
	enc := pem.EncodeToMemory(b)
	b1, _ := pem.Decode(enc)
	if b1 == nil {
		panic("can't decode encoded")
	}
	enc1 := pem.EncodeToMemory(b1)
	if !bytes.Equal(enc, enc1) {
		panic("encoded data differs")
	}
	return 1
}
