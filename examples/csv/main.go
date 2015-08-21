// Copyright 2015 Dmitry Vyukov. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

package csv

import (
	"bytes"
	"encoding/csv"
	"fmt"

	"github.com/dvyukov/go-fuzz/examples/fuzz"
)

func Fuzz(data []byte) int {
	score := 0

	r := csv.NewReader(bytes.NewReader(data))
	r.Comment = '#'
	r.LazyQuotes = true
	r.TrimLeadingSpace = true
	rec, err := r.ReadAll()
	if err != nil {
		if rec != nil {
			panic("rec is not nil on error")
		}
	} else {
		score = 1
	}

	r = csv.NewReader(bytes.NewReader(data))
	rec, err = r.ReadAll()
	if err != nil {
		if rec != nil {
			panic("rec is not nil on error")
		}
	} else {
		score = 1
		var rec0 [][]string
		for _, r := range rec {
			if len(r) > 0 {
				rec0 = append(rec0)
			}
		}

		buf := new(bytes.Buffer)
		w := csv.NewWriter(buf)
		w.WriteAll(rec0)
		r := csv.NewReader(buf)
		rec1, err := r.ReadAll()
		if err != nil {
			panic(err)
		}
		if !fuzz.DeepEqual(rec0, rec1) {
			fmt.Printf("rec0: %+v\n", rec0)
			fmt.Printf("rec1: %+v\n", rec1)
			panic("records differ")
		}
	}

	return score
}
