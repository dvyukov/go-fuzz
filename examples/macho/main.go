// Copyright 2015 Dmitry Vyukov. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

package macho

import (
	"bytes"
	"debug/macho"
)

func Fuzz(data []byte) int {
	f, err := macho.NewFile(bytes.NewReader(data))
	if err != nil {
		if f != nil {
			panic("file is not nil on error")
		}
		return 0
	}
	defer f.Close()
	f.ImportedLibraries()
	f.ImportedSymbols()
	f.Section(".text")
	f.Segment("macho")
	dw, err := f.DWARF()
	if err != nil {
		if dw != nil {
			panic("dwarf is not nil on error")
		}
		return 1
	}
	dr := dw.Reader()
	for {
		e, _ := dr.Next()
		if e == nil {
			break
		}
	}
	return 2
}
