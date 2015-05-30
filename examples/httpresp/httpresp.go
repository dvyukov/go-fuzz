package httpresp

import (
	"bufio"
	"bytes"
	"net/http"
)

func Fuzz(data []byte) int {
	r, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(data)), nil)
	if err != nil {
		return 0
	}
	r.Cookies()
	r.Location()
	return 1
}
