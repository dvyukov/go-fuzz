package aes

import (
	"bytes"
	cc "crypto/des"
)

func Fuzz(data []byte) int {
	const size = 8
	const block = 8
	if len(data) < size+2*block {
		return 0
	}
	c, err := cc.NewCipher(data[0:size])
	if err != nil {
		panic(err)
	}
	data1 := make([]byte, 2*block)
	c.Encrypt(data1[0:block], data[size:size+block])
	c.Encrypt(data1[block:2*block], data[size+block:size+2*block])
	data2 := make([]byte, 2*block)
	c.Decrypt(data2[0:block], data1[0:block])
	c.Decrypt(data2[block:2*block], data1[block:2*block])
	if !bytes.Equal(data[size:size+2*block], data2) {
		panic("data changed")
	}
	return 0
}
