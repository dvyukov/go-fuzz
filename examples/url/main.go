package url

import (
	"fmt"
	"net/url"
)

func Fuzz(data []byte) int {
	score := 0
	sdata := string(data)
	for _, parse := range []func(string) (*url.URL, error){url.Parse, url.ParseRequestURI} {
		url, err := parse(sdata)
		if err != nil {
			continue
		}
		score = 1
		sdata1 := url.String()
		url1, err := parse(sdata1)
		if err != nil {
			panic(err)
		}
		sdata2 := url1.String()
		if sdata1 != sdata2 {
			fmt.Printf("url0: %q\n", sdata1)
			fmt.Printf("url1: %q\n", sdata2)
			panic("url changed")
		}
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
