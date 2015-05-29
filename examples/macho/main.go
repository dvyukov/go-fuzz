package macho

import (
	"debug/macho"
	"bytes"
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
	dw, err := f.DWARF()
	if err != nil {
		if dw != nil {
			panic("dwarf is not nil on error")
		}
		return 1
	}
	
	return 2
}