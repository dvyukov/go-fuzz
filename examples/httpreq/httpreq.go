package httpreq

import (
	"bufio"
	"bytes"
	"net/http"
)

func Fuzz(data []byte) int {
	r, err := http.ReadRequest(bufio.NewReader(bytes.NewReader(data)))
	if err != nil {
		return 0
	}
	r.ParseForm()
	return 1
}
