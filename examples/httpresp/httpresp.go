package httpresp

import (
	"bufio"
	"bytes"
	"net/http"
)

func Fuzz(data []byte) int {
	_, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(data)), nil)
	if err != nil {
		return 0
	}
	return 1
}
