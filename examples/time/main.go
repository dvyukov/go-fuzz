package time

import "time"

func Fuzz(data []byte) int {
	var t time.Time
	if err := t.UnmarshalText(data); err != nil {
		return 0
	}
	_, err := t.MarshalText()
	if err != nil {
		panic(err)
	}
	return 1
}
