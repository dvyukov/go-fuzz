package xml

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"reflect"
)

type X struct {
	A int     `xml:"a,attr"`
	B float64 `xml:"B"`
	C bool    `xml:"C>CC"`
	D string  `xml:",comment,omitempty"`
	E []byte  `xml:",innerxml,omitempty"`
	F *X
	G string `xml:",any,omitempty"`
	H string `xml:"-,omitempty"`
	J []byte `xml:",chardata,omitempty"`
}

func Fuzz(data []byte) int {
	score := 0
	for _, ctor := range []func() interface{}{
		func() interface{} { return nil },
		func() interface{} { return new(string) },
		func() interface{} { return []string{} },
		func() interface{} { return new(X) },
	} {
		v := ctor()
		valid := false
		dec := xml.NewDecoder(bytes.NewReader(data))
		if dec.Decode(v) == nil {
			valid = true
		}
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
		if !reflect.DeepEqual(v, v1) {
			fmt.Printf("v0: %+v\n", v)
			fmt.Printf("v1: %+v\n", v)
			panic("non-idempotent Marshal/Unmarshal")
		}
	}
	return score
}
