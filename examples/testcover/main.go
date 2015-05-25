package testcover

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
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
	if int(n) <= len(data)-4 {
		s := string(data[4 : 4+n])
		if s == "eat this" {
			bingo()
		}
	}

	if f := binary.BigEndian.Uint32(data[9:]) > 0xfffffffd; f {
		bingo()
	}

	type Hdr struct {
		Magic [8]byte
		N     uint32
	}
	var hdr Hdr
	binary.Read(bytes.NewReader(data), binary.LittleEndian, &hdr)
	if hdr.Magic == [8]byte{'m', 'a', 'g', 'i', 'c', 'h', 'd', 'r'} {
		bingo()
	}

	type Name string
	name := Name(data[4:9])
	if name == "12345" {
		bingo()
	}

	if len(data) > 40 {
		hash1 := sha1.Sum(data[0:20])
		var hash2 [20]byte
		binary.Read(bytes.NewReader(data[20:40]), binary.LittleEndian, &hash2)
		if hash1 == hash2 {
			bingo()
		}
	}

	for i := 0; i < 6; i++ {
		if data[i] != "CDATA["[i] {
			goto fail
		}
	}
	bingo()
fail:

	if varx, _ := binary.Uvarint(data[3:]); varx == 0xbadbeefc0ffee {
		bingo()
	}

	if data, err := hex.DecodeString(string(data[:6])); err == nil && string(data) == "foo" {
		bingo()
	}

        if data[0] != 'R' || data[1] != 'I' || data[2] != 'F' || data[3] != 'F' {
		bingo()
        }

	if x := binary.LittleEndian.Uint32(data[8:]); (x>>1)&(1<<24-1) == 11118709 {
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
