package gofuzzdep

import (
	"syscall"
)

type FD int

func setupCommFile() ([]byte, FD, FD) {
	return nil, 3, 4
}
