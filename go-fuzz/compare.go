// +build !amd64

package main

func compareCoverBody(base, cur []byte) bool {
	return compareCoverDump(base, cur)
}
