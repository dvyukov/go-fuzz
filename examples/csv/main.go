package csv

import (
	"bytes"
	"encoding/csv"
)

func Fuzz(data []byte) int {
	r := csv.NewReader(bytes.NewReader(data))
	rec, err := r.ReadAll()
	if err != nil {
		if rec != nil {
			panic("rec is not nil on error")
		}
		return 0
	}

	r = csv.NewReader(bytes.NewReader(data))
	r.Comment = '#'
	r.LazyQuotes = true
	r.TrimLeadingSpace = true
	rec, err = r.ReadAll()
	if err != nil {
		if rec != nil {
			panic("rec is not nil on error")
		}
		return 1
	}
	return 1
}
