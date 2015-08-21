// Copyright 2015 Dmitry Vyukov. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

package testcover

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
	"strings"
	"unicode"
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
	if unicode.ToLower(rune(data[0])) != 'a' &&
		unicode.ToLower(rune(data[1])) != 'b' &&
		unicode.ToLower(rune(data[2])) != 'c' &&
		unicode.ToLower(rune(data[3])) != 'd' &&
		unicode.ToLower(rune(data[4])) != 'e' &&
		unicode.ToLower(rune(data[5])) != 'f' &&
		unicode.ToLower(rune(data[6])) != 'g' &&
		unicode.ToLower(rune(data[7])) != 'h' {
		bingo()
	}

	if x := binary.LittleEndian.Uint32(data[8:]); (x>>1)&(1<<24-1) == 11118709 {
		bingo()
	}

	if strings.HasPrefix(string(data[2:]), "some prefix") {
		bingo()
	}
	if strings.HasSuffix(string(data[3:]), "some suffix") {
		bingo()
	}
	if strings.Index(string(data[4:]), "some index") != -1 {
		bingo()
	}
	if strings.IndexByte(string(data[4:]), 'X') != -1 {
		bingo()
	}
	if strings.Contains(string(data[1:]), "contained") {
		bingo()
	}
	if strings.ToLower(string(data[2:])) == "lower substr" {
		bingo()
	}
	if strings.ToUpper(string(data[2:])) == "UPPER SUBSTR" {
		bingo()
	}
	if strings.HasPrefix(strings.ToUpper(string(data[2:])), "UPPER PREFIX") {
		bingo()
	}

	if bytes.HasPrefix(data[3:], []byte("some prefix")) {
		bingo()
	}
	if bytes.HasSuffix(data[:len(data)-1], []byte("some suffix")) {
		bingo()
	}
	if bytes.Index(data[2:], []byte("bytes index")) != -1 {
		bingo()
	}
	if bytes.IndexByte(data[4:], 'Y') != -1 {
		bingo()
	}
	if bytes.Contains(data[1:], []byte("magic word")) {
		bingo()
	}
	if bytes.HasSuffix(bytes.ToUpper(data[:len(data)-1]), []byte("UPPER BYTE")) {
		bingo()
	}
	if bytes.Equal(bytes.ToLower(data[:11]), []byte("lower equal")) {
		bingo()
	}
	if bytes.Index(bytes.ToUpper(data[2:]), []byte("lower index")) != -1 {
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
