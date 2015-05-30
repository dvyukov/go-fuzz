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
	r.ParseMultipartForm(1e6)
	r.ParseForm()
	r.BasicAuth()
	r.Cookies()
	r.FormFile("foo")
	r.FormValue("bar")
	r.PostFormValue("baz")
	r.MultipartReader()
	r.Referer()
	r.UserAgent()
	return 1
}
