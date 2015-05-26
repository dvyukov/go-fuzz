package tar

import (
	"archive/tar"
	"bytes"
	"io"
	"io/ioutil"
)

func Fuzz(data []byte) int {
	t := tar.NewReader(bytes.NewReader(data))
	score := 0
	for {
		_, err := t.Next()
		if err != nil {
			return score
		}
		io.Copy(ioutil.Discard, t)
		score++
	}
}
