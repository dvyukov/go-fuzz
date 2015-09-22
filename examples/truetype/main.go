package truetype

import (
	"github.com/golang/freetype/truetype"
)

func Fuzz(data []byte) int {
	f, err := truetype.Parse(data)
	if err != nil {
		if f != nil {
			panic("font is not nil on error")
		}
		return 0
	}
	return 1
}
