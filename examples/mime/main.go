package mime

import (
	"fmt"
	"mime"

	"github.com/dvyukov/go-fuzz/examples/fuzz"
)

func Fuzz(data []byte) int {
	sdata := string(data)
	mt, params, err := mime.ParseMediaType(sdata)
	if err != nil {
		return 0
	}
	sdata1 := mime.FormatMediaType(mt, params)
	mt1, params1, err := mime.ParseMediaType(sdata1)
	if err != nil {
		if err.Error() == "mime: no media type" {
			// https://github.com/golang/go/issues/11289
			return 0
		}
		if err.Error() == "mime: invalid media parameter" {
			// https://github.com/golang/go/issues/11290
			return 0
		}
		fmt.Printf("%q(%q, %+v) -> %q\n", sdata, mt, params, sdata1)
		panic(err)
	}
	if !fuzz.DeepEqual(mt, mt1) {
		fmt.Printf("%q -> %q\n", mt, mt1)
		panic("mediatype changed")
	}
	if !fuzz.DeepEqual(params, params1) {
		fmt.Printf("%+v -> %+v\n", params, params1)
		panic("params changed")
	}
	return 1
}
