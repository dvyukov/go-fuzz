// Copyright 2015 Dmitry Vyukov. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

package xml

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"reflect"

	"github.com/dvyukov/go-fuzz/examples/fuzz"
)

type X struct {
	XMLName xml.Name
	A       int     `xml:"a,attr"`
	B       float64 `xml:"B"`
	C       bool
	C1      bool   `xml:"C>CC"`
	D       string `xml:",comment,omitempty"`
	E       []byte
	E1      []byte `xml:",innerxml,omitempty"`
	F       *Y
	G       string `xml:",any,omitempty"`
	H       string `xml:"-,omitempty"`
	J       []byte `xml:",chardata,omitempty"`
	K       **int
}

type Y struct {
	XMLName xml.Name `xml:"http://www.google.com/test name"`
	A       string   `xml:",chardata"`
	B       int      `xml:"-"`
}

func Fuzz(data []byte) int {
	score := 0
	for _, ctor := range []func() interface{}{
		func() interface{} { return nil },
		func() interface{} { return new(string) },
		func() interface{} { return new([]string) },
		func() interface{} { return new(X) },
		func() interface{} { return new([]X) },
	} {
		v0 := ctor()
		valid := false
		dec := xml.NewDecoder(bytes.NewReader(data))
		if dec.Decode(v0) == nil {
			valid = true
		}
		v := ctor()
		dec1 := xml.NewDecoder(bytes.NewReader(data))
		dec1.Strict = false
		dec1.AutoClose = xml.HTMLAutoClose
		dec1.Entity = xml.HTMLEntity
		if dec1.Decode(v) != nil {
			if valid {
				panic("non-strict mode is weaker than strict")
			}
			continue
		}
		score = 1
		data1, err := xml.Marshal(v)
		if err != nil {
			panic(err)
		}

		v1 := ctor()
		dec2 := xml.NewDecoder(bytes.NewReader(data1))
		dec2.Strict = false
		dec2.AutoClose = xml.HTMLAutoClose
		dec2.Entity = xml.HTMLEntity
		if err := dec2.Decode(v1); err != nil {
			panic(err)
		}
		if !fuzz.DeepEqual(v, v1) {
			fmt.Printf("v0: %#v\n", reflect.ValueOf(v).Elem().Interface())
			fmt.Printf("v1: %#v\n", reflect.ValueOf(v1).Elem().Interface())
			panic(fmt.Sprintf("not equal %T", v))
		}
	}
	return score
}
