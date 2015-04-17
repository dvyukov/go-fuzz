package nss

import (
	"bytes"
	"net"
)

func Fuzz(data []byte) int {
	if net.ParseNSSConf(bytes.NewReader(data)) == nil {
		return 0
	}
	return 1
}
