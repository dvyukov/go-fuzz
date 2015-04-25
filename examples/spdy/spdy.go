// golang.org/x/net/spdy package was deleted (in part due to a pile of bugs discovered by go-fuzz)
// +build never

package spdy

import (
	"bytes"
	"golang.org/x/net/spdy"
	"io/ioutil"
)

func Fuzz(data []byte) int {
	framer, err := spdy.NewFramer(ioutil.Discard, bytes.NewReader(data))
	if err != nil {
		panic(err)
	}
	for score := 0; ; score++ {
		f, err := framer.ReadFrame()
		if err != nil {
			if f != nil {
				panic(err)
			}
			return score
		}
		err = framer.WriteFrame(f)
		if err != nil {
			panic(err)
		}
	}
}
