package gob

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"reflect"
)

type X struct {
	A int
	B string
	C float64
	D []byte
	E interface{}
	F complex128
	G []interface{}
	H *int
	I **int
	J *X
	K map[string]int
}

func init() {
	gob.Register(X{})
}

func Fuzz(data []byte) int {
	score := 0
	for _, ctor := range []func() interface{}{
		func() interface{} { return nil },
		func() interface{} { return new(int) },
		func() interface{} { return new(string) },
		func() interface{} { return new(float64) },
		func() interface{} { return []byte{} },
		func() interface{} { return new([]byte) },
		func() interface{} { return []interface{}{} },
		func() interface{} { return new(interface{}) },
		func() interface{} { return new(complex128) },
		func() interface{} { return make(map[int]int) },
		func() interface{} { return make(map[string]interface{}) },
		func() interface{} { return new(X) },
	} {
		v := ctor()
		if gob.NewDecoder(bytes.NewReader(data)).Decode(v) != nil {
			continue
		}
		score = 1
		if ctor() == nil {
			continue
		}
		b1 := new(bytes.Buffer)
		if err := gob.NewEncoder(b1).Encode(v); err != nil {
			panic(err)
		}
		v1 := reflect.ValueOf(ctor())
		err := gob.NewDecoder(bytes.NewReader(data)).DecodeValue(v1)
		if err != nil {
			panic(err)
		}
		if !reflect.DeepEqual(v, v1.Interface()) {
			fmt.Printf("v0: %#v\n", v)
			fmt.Printf("v1: %#v\n", v1.Interface())
			panic("values not equal")
		}
		b2 := new(bytes.Buffer)
		err = gob.NewEncoder(b2).EncodeValue(v1)
		if err != nil {
			panic(err)
		}
		if !bytes.Equal(b1.Bytes(), b2.Bytes()) {
			panic("bytes are not equal")
		}
		v2 := ctor()
		if err := gob.NewDecoder(b1).Decode(v2); err != nil {
			panic(err)
		}
		if !reflect.DeepEqual(v, v2) {
			fmt.Printf("v0: %#v\n", v)
			fmt.Printf("v2: %#v\n", v2)
			panic("values not equal 2")
		}
	}
	return score
}
