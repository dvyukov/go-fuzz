package url

import (
	"net/url"
)

func Fuzz(data []byte) int {
	sdata := string(data)
	score := 0
	if _, err := url.Parse(sdata); err == nil {
		score++
	}
	if _, err := url.ParseRequestURI(sdata); err == nil {
		score++
	}
	return score
}

func FuzzValues(data []byte) int {
	_, err := url.ParseQuery(string(data))
	if err != nil {
		return 0
	}
	return 1
}
