package gob

import (
	"encoding/gob"
	"bytes"
)

func Fuzz(data []byte) int {
	r := bytes.NewReader(data)
	g := gob.NewDecoder(r)
	if g.Decode(nil) != nil {
		return 0
	}
	if g.Decode(nil) != nil {
		return 1
	}
	if g.Decode(nil) != nil {
		return 2
	}
	if g.Decode(nil) != nil {
		return 3
	}
	if g.Decode(nil) != nil {
		return 4
	}
	return 5
}
