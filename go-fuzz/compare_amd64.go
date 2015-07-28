package main

func compareCoverBody(base, cur []byte) bool {
	return compareCoverBody1(&base[0], &cur[0])
}

func compareCoverBody1(base, cur *byte) bool // in compare_amd64.s
