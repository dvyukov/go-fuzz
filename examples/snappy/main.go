package snappy

import (
	"github.com/golang/snappy/snappy"
)

func Fuzz(data []byte) int {
	n, err := snappy.DecodedLen(data)
	if err != nil || n > 1e6 {
		return 0
	}
	if n < 0 {
		panic("negative decoded len")
	}
	dec, err := snappy.Decode(nil, data)
	if err != nil {
		if dec != nil {
			panic("dec is not nil")
		}
		return 0
	}
	if len(dec) != n {
		println(len(dec), n)
		panic("bad decoded len")
	}
	n = snappy.MaxEncodedLen(len(dec))
	enc, err := snappy.Encode(nil, dec)
	if err != nil {
		panic(err)
	}
	if len(enc) > n {
		panic("bad encoded len")
	}
	return 1
}
