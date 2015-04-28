package x509

import (
	"crypto/x509"
)

func Fuzz(data []byte) int {
	list, err := x509.ParseCRL(data)
	if err != nil {
		if list != nil {
			panic("list is not nil on error")
		}
		return 0
	}
	return 1

}
