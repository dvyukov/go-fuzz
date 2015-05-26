package zip

import (
	"archive/zip"
	"bytes"
	"io"
	"io/ioutil"
)

func Fuzz(data []byte) int {
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		if z != nil {
			panic("non nil z")
		}
		return 0
	}
	for _, f := range z.File {
		r, err := f.Open()
		if err != nil {
			continue
		}
		if f.UncompressedSize64 < 1e6 {
			n, err := io.Copy(ioutil.Discard, r)
			if err == nil && uint64(n) != f.UncompressedSize64 {
				println("bad size:", n, f.UncompressedSize64)
				panic("bad size")
			}
		}
		r.Close()
	}
	return 1
}
