package gofuzzdep

import (
	"sync/atomic"
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
		buf[0] = byte(vv >> 0)
		buf[1] = byte(vv >> 8)
		return 2
	case uint16:
		buf[0] = byte(vv >> 0)
		buf[1] = byte(vv >> 8)
		return 2
	case int32:
		buf[0] = byte(vv >> 0)
		buf[1] = byte(vv >> 8)
		buf[2] = byte(vv >> 16)
		buf[3] = byte(vv >> 24)
		return 4
	case uint32:
		buf[0] = byte(vv >> 0)
		buf[1] = byte(vv >> 8)
		buf[2] = byte(vv >> 16)
		buf[3] = byte(vv >> 24)
		return 4
	case int64:
		buf[0] = byte(vv >> 0)
		buf[1] = byte(vv >> 8)
		buf[2] = byte(vv >> 16)
		buf[3] = byte(vv >> 24)
		buf[4] = byte(vv >> 32)
		buf[5] = byte(vv >> 40)
		buf[6] = byte(vv >> 48)
		buf[7] = byte(vv >> 56)
		return 8
	case uint64:
		buf[0] = byte(vv >> 0)
		buf[1] = byte(vv >> 8)
		buf[2] = byte(vv >> 16)
		buf[3] = byte(vv >> 24)
		buf[4] = byte(vv >> 32)
		buf[5] = byte(vv >> 40)
		buf[6] = byte(vv >> 48)
		buf[7] = byte(vv >> 56)
		return 8
	case string:
		if len(vv) > SonarMaxLen {
			return 0
		}
		return uint8(copy(buf, vv))
	default:
		return 0
	}
}
