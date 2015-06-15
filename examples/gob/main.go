package gob

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"io"
	"reflect"

	"github.com/dvyukov/go-fuzz/examples/fuzz"
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
		func() interface{} { return new([]byte) },
		func() interface{} { return new(interface{}) },
		func() interface{} { return new(complex128) },
		func() interface{} { m := make(map[int]int); return &m },
		func() interface{} { m := make(map[string]interface{}); return &m },
		func() interface{} { return new(X) },
	} {
		v := ctor()
		dec := gob.NewDecoder(bytes.NewReader(data))
		if dec.Decode(v) != nil {
			continue
		}
		dec.Decode(ctor())
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
		if !fuzz.DeepEqual(v, v1.Interface()) {
			fmt.Printf("v0: %#v\n", reflect.ValueOf(v).Elem().Interface())
			fmt.Printf("v1: %#v\n", v1.Elem().Interface())
			panic(fmt.Sprintf("values not equal %T", v))
		}
		b2 := new(bytes.Buffer)
		err = gob.NewEncoder(b2).EncodeValue(v1)
		if err != nil {
			panic(err)
		}
		v2 := ctor()
		dec1 := gob.NewDecoder(b1)
		if err := dec1.Decode(v2); err != nil {
			panic(err)
		}
		if err := dec1.Decode(ctor()); err != io.EOF {
			panic(err)
		}
		if vv, ok := v.(*X); ok {
			fix(vv)
		}
		if !fuzz.DeepEqual(v, v2) {
			fmt.Printf("v0: %#v\n", reflect.ValueOf(v).Elem().Interface())
			fmt.Printf("v2: %#v\n", reflect.ValueOf(v2).Elem().Interface())
			panic(fmt.Sprintf("values not equal 2 %T", v))
		}
	}
	return score
}

func fix(vv *X) {
	// See https://github.com/golang/go/issues/11119
	if vv.I != nil && (*vv.I == nil || **vv.I == 0) {
		// If input contains "I:42 I:null", then I will be in this weird state.
		// It is effectively nil, but DeepEqual does not handle such case.
		vv.I = nil
	}
	if vv.H != nil && *vv.H == 0 {
		vv.H = nil
	}
	if vv.J != nil {
		fix(vv.J)
	}
}
