package xml

import (
	"bytes"
	"encoding/xml"
)

type X struct {
	A int     `xml:"a,attr"`
	B float64 `xml:"B"`
	C bool    `xml:"C>CC"`
	D string  `xml:",comment"`
	E []byte  `xml:",innerxml"`
	F *X
	G string `xml:",any"`
	H string `xml:"-"`
	J []byte `xml:",chardata"`
}

func Fuzz(data []byte) int {
	score := 0
	if test(data, nil) {
		score = 1
	}
	var s string
	if test(data, &s) {
		score = 1
	}
	var a []string
	if test(data, &a) {
		score = 1
	}
	var x X
	if test(data, &x) {
		score = 1
		xml.Marshal(&x)
	}
	return score
}

func test(data []byte, v interface{}) (valid bool) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	if dec.Decode(v) == nil {
		valid = true
	}
	dec1 := xml.NewDecoder(bytes.NewReader(data))
	dec1.Strict = false
	dec1.AutoClose = xml.HTMLAutoClose
	dec1.Entity = xml.HTMLEntity
	if dec1.Decode(v) == nil {
		valid = true
	} else if valid {
		panic("non-strict mode is weaker than strict")
	}
	return
}
