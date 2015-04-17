package regexp

import (
	"regexp"
)

func Fuzz(data []byte) int {
	restr := string(data[len(data)/2:])
	score := 0
	re, err := regexp.Compile(restr)
	if err == nil {
		res := re.FindAllSubmatch(data[:len(data)/2], 100)
		score += len(res)
	}
	rep, err := regexp.CompilePOSIX(restr)
	if err == nil {
		res := rep.FindAllSubmatch(data[:len(data)/2], 100)
		score += len(res)
	}
	return score
}
