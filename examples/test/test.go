package test

import (
	"bytes"
)

func Fuzz(data []byte) int {
	if len(data) == 1 {
		if data[0] == '!' || data[0] == '#' {
			panic("bingo 0")
		}
		if data[0] == '0' || data[0] == '9' {
			for {
				c := make(chan bool)
				close(c)
			}
		}
		if data[0] == 'a' || data[0] == 'z' {
			data := make([]byte, 128<<30-1)
			_ = data
		}
		if data[0] == 'b' {
			// new coverage
		}
		if data[0] == 'c' {
			// new coverage
		}
	}

	// Test for crash minimization.
	if bytes.IndexByte(data, 'x') != -1 {
		if bytes.IndexByte(data, 'y') != -1 {
			panic("xy")
		}
	}

	// Test for input minimization.
	if bytes.Index(data, []byte("input ")) != -1 {
		if bytes.Index(data, []byte("minimization ")) != -1 {
			if bytes.Index(data, []byte("test")) != -1 {
			}
		}
	}

	if len(data) >= 14 && bytes.HasPrefix(data, []byte("0123456789")) {
		x := int(data[10]) + int(data[11])<<8 + int(data[12])<<16 + int(data[13])<<24
		if x == 0 || x == -1 {
			panic("bingo 1")
		}
		if x == 255 || x == 256 {
			for {
				c := make(chan bool)
				close(c)
			}
		}
		if x == 1<<16-1 || x == 1<<16 {
			data := make([]byte, 128<<30-1)
			_ = data
		}
		if x == '1' {
			// new coverage
		}
		if x == '2' {
			// new coverage
		}
	}
	return 0
}
