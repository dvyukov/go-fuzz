// Copyright 2015 Dmitry Vyukov. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

package bzip2

import (
	"bytes"
	"compress/bzip2"
	"io/ioutil"
)

func Fuzz(data []byte) int {
	_, err := ioutil.ReadAll(bzip2.NewReader(bytes.NewReader(data)))
	if err != nil {
		return 0
	}
	return 1
}
