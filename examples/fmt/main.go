// Copyright 2015 Dmitry Vyukov. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

package fmt

import (
	"fmt"
	"reflect"
)

func Fuzz(data []byte) int {
	f := string(data[:len(data)/2])
	s := string(data[len(data)/2:])
	for _, ctor := range []func() interface{}{
		func() interface{} { return new(int) },
		func() interface{} { return new(byte) },
		func() interface{} { return new(uint16) },
		func() interface{} { return new(int16) },
		func() interface{} { return new(string) },
		func() interface{} { return new(float32) },
		func() interface{} { return new(float64) },
		func() interface{} { return new(string) },
		func() interface{} { return new(complex128) },
		func() interface{} { return new([]int) },
		func() interface{} { return new([][]string) },
		func() interface{} { m := make(map[int]float64); return &m },
		func() interface{} {
			return &struct {
				A int
				B float64
				C string
				D []int
			}{}
		},
	} {
		v := ctor()
		fmt.Sscanf(s, f, v)
		fmt.Sprintf(f, reflect.ValueOf(v).Elem().Interface())
	}
	return 0
}
