package gofuzzdep

import (
	"sync/atomic"
	"unsafe"
)

const (
	SonarEQL = iota
	SonarNEQ
	SonarLSS
	SonarGTR
	SonarLEQ
	SonarGEQ

	SonarString = 1 << 5
	SonarConst1 = 1 << 6
	SonarConst2 = 1 << 7

	SonarHdrLen = 3
	SonarMaxLen = 20
)

func Sonar(v1, v2 interface{}, flags uint8) {
	var buf [SonarHdrLen + 2*SonarMaxLen]byte
	if buf[1] = serialize(v1, buf[SonarHdrLen:]); buf[1] == 0 {
		return
	}
	if buf[2] = serialize(v2, buf[SonarHdrLen+buf[1]:]); buf[2] == 0 {
		return
	}
	if _, ok := v1.(string); ok {
		flags |= SonarString
	}
	if _, ok := v2.(string); ok {
		flags |= SonarString
	}
	buf[0] = flags
	n := uint32(SonarHdrLen + buf[1] + buf[2])
	pos := atomic.LoadUint32(&sonarPos)
	for {
		if pos+n > uint32(len(sonarRegion)) {
			return
		}
		if atomic.CompareAndSwapUint32(&sonarPos, pos, pos+n) {
			break
		}
		pos = atomic.LoadUint32(&sonarPos)
	}
	copy(sonarRegion[pos:pos+n], buf[:])
}

func serialize(v interface{}, buf []byte) uint8 {
	switch vv := v.(type) {
	case int8:
		buf[0] = byte(vv)
		return 1
	case uint8:
		buf[0] = byte(vv)
		return 1
	case int16:
		return serialize16(buf, uint16(vv))
	case uint16:
		return serialize16(buf, vv)
	case int32:
		return serialize32(buf, uint32(vv))
	case uint32:
		return serialize32(buf, vv)
	case int64:
		return serialize64(buf, uint64(vv))
	case uint64:
		return serialize64(buf, vv)
	case int:
		if unsafe.Sizeof(vv) == 4 {
			return serialize32(buf, uint32(vv))
		} else {
			return serialize64(buf, uint64(vv))
		}
	case uint:
		if unsafe.Sizeof(vv) == 4 {
			return serialize32(buf, uint32(vv))
		} else {
			return serialize64(buf, uint64(vv))
		}
	case string:
		if len(vv) > SonarMaxLen {
			return 0
		}
		return uint8(copy(buf, vv))
	default:
		return 0
	}
}

func serialize16(buf []byte, v uint16) uint8 {
	buf[0] = byte(v >> 0)
	buf[1] = byte(v >> 8)
	return 2
}

func serialize32(buf []byte, v uint32) uint8 {
	buf[0] = byte(v >> 0)
	buf[1] = byte(v >> 8)
	buf[2] = byte(v >> 16)
	buf[3] = byte(v >> 24)
	return 4
}

func serialize64(buf []byte, v uint64) uint8 {
	buf[0] = byte(v >> 0)
	buf[1] = byte(v >> 8)
	buf[2] = byte(v >> 16)
	buf[3] = byte(v >> 24)
	buf[4] = byte(v >> 32)
	buf[5] = byte(v >> 40)
	buf[6] = byte(v >> 48)
	buf[7] = byte(v >> 56)
	return 8
}

func deserialize64(buf []byte) uint64 {
	return uint64(buf[0])<<0 |
		uint64(buf[1])<<8 |
		uint64(buf[2])<<16 |
		uint64(buf[3])<<24 |
		uint64(buf[4])<<32 |
		uint64(buf[5])<<40 |
		uint64(buf[6])<<48 |
		uint64(buf[7])<<56
}
