package asn1

import (
	"encoding/asn1"
	"fmt"
	"math/big"
	"reflect"
	"time"
)

func Fuzz(data []byte) int {
	ctors := []func() interface{}{
		func() interface{} { return new(int) },
		func() interface{} { return new(int32) },
		func() interface{} { return new(int64) },
		func() interface{} { return new(*big.Int) },
		func() interface{} { return new(asn1.BitString) },
		func() interface{} { return new([]byte) },
		func() interface{} { return new(asn1.ObjectIdentifier) },
		func() interface{} { return new(asn1.Enumerated) },
		func() interface{} { return new(interface{}) },
		func() interface{} { return new(string) },
		func() interface{} { return new(time.Time) },
		func() interface{} { return new([]interface{}) },
		func() interface{} { return new(X) },
	}
	score := 0
	for _, ctor := range ctors {
		v := ctor()
		_, err := asn1.Unmarshal(data, v)
		if err != nil {
			continue
		}
		score = 1
		x := reflect.ValueOf(v).Elem().Interface()
		if x == nil {
			continue // https://github.com/golang/go/issues/11127
		}
		data1, err := asn1.Marshal(x)
		if err != nil {
			if err.Error() == "asn1: structure error: cannot represent time as GeneralizedTime" {
				continue
			}
			panic(err)
		}
		v1 := ctor()
		rest, err := asn1.Unmarshal(data1, v1)
		if err != nil {
			panic(err)
		}
		if len(rest) != 0 {
			fmt.Printf("data: %q\n", rest)
			panic("leftover data")
		}
		if !reflect.DeepEqual(v, v1) {
			fmt.Printf("v0: %#v\n", v)
			fmt.Printf("v1: %#v\n", v1)
			panic(fmt.Sprintf("not equal %T", x))
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
	A  int
	A1 int `asn1:"application"`
	A2 int `asn1:"optional"`
	A3 int `asn1:"optional,default:123"`
	A4 int `asn1:"explicit"`
	A5 int `asn1:"tag:0"`
	A6 int `asn1:"tag:5"`
	B  int32
	C  int64
	D  *big.Int
	E  asn1.BitString
	F  []byte
	F1 []byte `asn1:"omitempty"`
	F2 []byte `asn1:"set"`
	F3 []int
	F4 []int `asn1:"set"`
	G  asn1.ObjectIdentifier
	H  asn1.Enumerated
	I  interface{}
	S  string
	S1 string `asn1:"ia5"`
	S2 string `asn1:"printable"`
	S3 string `asn1:"utf8"`
	S4 string `asn1:"omitempty"`
	S5 string `asn1:"application"`
	T  time.Time
	Y  YY
	Z  []interface{}
}
