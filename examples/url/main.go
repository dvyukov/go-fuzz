// Copyright 2015 Dmitry Vyukov. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

package url

import (
	"fmt"
	"net/url"
	"reflect"
)

func Fuzz(data []byte) int {
	score := 0
	sdata := string(data)
	for _, parse := range []func(string) (*url.URL, error){url.Parse, url.ParseRequestURI} {
		url, err := parse(sdata)
		if err != nil {
			continue
		}
		score = 1
		sdata1 := url.String()
		url1, err := parse(sdata1)
		if err != nil {
			panic(err)
		}
		sdata2 := url1.String()
		if sdata1 != sdata2 {
			fmt.Printf("url0: %q\n", sdata1)
			fmt.Printf("url1: %q\n", sdata2)
			panic("url changed")
		}
	}
	return score
}

func FuzzValues(data []byte) int {
	sdata := string(data)
	vals, err := url.ParseQuery(sdata)
	if err != nil {
		return 0
	}
	sdata1 := vals.Encode()
	vals1, err := url.ParseQuery(sdata1)
	if err != nil {
		panic(err)
	}
	if !reflect.DeepEqual(vals, vals1) {
		fmt.Printf("vals0: %#v\n", vals)
		fmt.Printf("vals1: %#v\n", vals1)
		panic("bad")
	}
	return 1
}
