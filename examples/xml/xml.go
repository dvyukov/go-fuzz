package xml

import (
	"encoding/xml"
)

type X struct {
	A int
	B float64
	C bool
	D string
	E []byte
	F *X
}

func Fuzz(data []byte) int {
	score := 0
	if xml.Unmarshal(data, nil) == nil {
		score++
	}
	var s string
	if xml.Unmarshal(data, &s) == nil {
		score++
	}
	var a []string
	if xml.Unmarshal(data, &a) == nil {
		score++
	}
	var x X
	if xml.Unmarshal(data, &x) == nil {
		score++
	}
	return score
}
