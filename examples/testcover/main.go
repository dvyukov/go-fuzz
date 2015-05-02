package testcover

import (
	"encoding/binary"
)

func Fuzz(data []byte) int {
	if len(data) < 20 {
		return 0
	}
	x := binary.BigEndian.Uint32(data[12:])
	if x == 0x45839281 {
		bingo()
	}
	if data[10] == 0xfd && data[15] == 0x9a && data[17] == 0x71 {
		bingo()
	}
	switch binary.LittleEndian.Uint32(data[10:]) {
	default:
		bingo()
	case 0x12345678:
		bingo()
	case 0x98765432:
		bingo()
	}
	switch {
	case binary.LittleEndian.Uint32(data[8:]) == 0x12345678:
		bingo()
	default:
		bingo()
	case 0x98765432 == binary.BigEndian.Uint32(data[7:]):
		bingo()
	}

	switch string(data[5:9]) {
	case "ABCD":
		bingo()
	case "QWER":
		bingo()
	case "ZXCV":
		bingo()
	}

	n := binary.BigEndian.Uint32(data[0:4])
	if int(n) <= len(data) - 4 {
		s := string(data[4:4+n])
		if s == "eat this" {
			bingo()
		}
	}


	if f := binary.BigEndian.Uint32(data[9:]) > 0xfffffffd; f {
		bingo()
	}

	return 0
}

func bingo() {
	if theFalse {
		bingo()
	}
}

var theFalse = false
