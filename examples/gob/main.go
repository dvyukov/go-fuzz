package gob

import (
	"bytes"
	"encoding/gob"
	"reflect"
)

type X struct {
	A int
	B string
	C float64
	D []byte
	I interface{}
}

func init() {
	gob.Register(X{})
}

func Fuzz(data []byte) int {
	score := 0
	if gob.NewDecoder(bytes.NewReader(data)).Decode(nil) == nil {
		score++
	}
	i := 0
	if gob.NewDecoder(bytes.NewReader(data)).Decode(&i) == nil {
		score++
	}
	s := ""
	if gob.NewDecoder(bytes.NewReader(data)).Decode(&s) == nil {
		score++
	}
	var b []byte
	if gob.NewDecoder(bytes.NewReader(data)).Decode(&b) == nil {
		score++
	}
	var f float64
	if gob.NewDecoder(bytes.NewReader(data)).Decode(&f) == nil {
		score++
	}
	var x X
	if gob.NewDecoder(bytes.NewReader(data)).Decode(&x) == nil {
		score++
	}
	var v reflect.Value
	if gob.NewDecoder(bytes.NewReader(data)).Decode(&v) == nil {
		score++
	}
	if gob.NewDecoder(bytes.NewReader(data)).DecodeValue(v) == nil {
		score++
	}
	return score
}
