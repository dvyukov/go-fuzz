package asn1

import (
	"encoding/asn1"
	"math/big"
	"time"
)

func Fuzz(data []byte) int {
	elems := []interface{}{
		new(int),
		new(int32),
		new(int64),
		new(*big.Int),
		new(asn1.BitString),
		new([]byte),
		new(asn1.ObjectIdentifier),
		new(asn1.Enumerated),
		new(interface{}),
		new(string),
		new(time.Time),
		new([]interface{}),
		new(X),
	}
	score := 0
	for _, e := range elems {
		_, err := asn1.Unmarshal(data, e)
		if err != nil {
			score = 1
		}
	}
	return score
}

type YY struct {
	A int
	B int32
	C int64
}

type X struct {
	A int
	B int32
	C int64
	D *big.Int
	E asn1.BitString
	F []byte
	G asn1.ObjectIdentifier
	H asn1.Enumerated
	I interface{}
	S string
	T time.Time
	Y YY
	Z []interface{}
}
