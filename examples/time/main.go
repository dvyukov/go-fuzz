// Copyright 2015 Dmitry Vyukov. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

package time

import (
	"fmt"
	"time"

	"github.com/dvyukov/go-fuzz/examples/fuzz"
)

func Fuzz(data []byte) int {
	var t time.Time
	if err := t.UnmarshalText(data); err != nil {
		return 0
	}
	data1, err := t.MarshalText()
	if err != nil {
		panic(err)
	}
	var t1 time.Time
	if err := t1.UnmarshalText(data1); err != nil {
		panic(err)
	}
	if !fuzz.DeepEqual(t, t1) {
		fmt.Printf("t0: %#v\n", t)
		fmt.Printf("t1: %#v\n", t1)
		panic("bad MarshalText")
	}

	data2, err := t.GobEncode()
	if err != nil {
		panic(err)
	}
	var t2 time.Time
	if err := t2.GobDecode(data2); err != nil {
		panic(err)
	}
	if !fuzz.DeepEqual(t, t2) {
		fmt.Printf("t0: %#v\n", t)
		fmt.Printf("t2: %#v\n", t2)
		panic("bad GobEncode")
	}

	data3, err := t.MarshalBinary()
	if err != nil {
		panic(err)
	}
	var t3 time.Time
	if err := t3.UnmarshalBinary(data3); err != nil {
		panic(err)
	}
	if !fuzz.DeepEqual(t, t3) {
		fmt.Printf("t0: %#v\n", t)
		fmt.Printf("t3: %#v\n", t3)
		panic("bad MarshalBinary")
	}

	data4, err := t.MarshalJSON()
	if err != nil {
		panic(err)
	}
	var t4 time.Time
	if err := t4.UnmarshalJSON(data4); err != nil {
		panic(err)
	}
	if !fuzz.DeepEqual(t, t4) {
		fmt.Printf("t0: %#v\n", t)
		fmt.Printf("t4: %#v\n", t4)
		panic("bad MarshalJSON")
	}

	data5, err := t.MarshalText()
	if err != nil {
		panic(err)
	}
	var t5 time.Time
	if err := t5.UnmarshalText(data5); err != nil {
		panic(err)
	}
	if !fuzz.DeepEqual(t, t5) {
		fmt.Printf("t0: %#v\n", t)
		fmt.Printf("t5: %#v\n", t5)
		panic("bad MarshalText")
	}

	data6 := t.Format(time.RFC3339Nano)
	t6, err := time.Parse(time.RFC3339Nano, data6)
	if err != nil {
		panic(err)
	}
	if !fuzz.DeepEqual(t, t6) {
		fmt.Printf("t0: %#v\n", t)
		fmt.Printf("t6: %#v\n", t6)
		panic("bad Format")
	}
	return 1
}
