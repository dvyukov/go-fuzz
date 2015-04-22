package json

import (
	"encoding/json"
)

func Fuzz(data []byte) int {
	score := 0
	{
		var v interface{}
		if json.Unmarshal(data, &v) == nil {
			score++
		}
	}
	{
		var v []interface{}
		if json.Unmarshal(data, &v) == nil {
			score++
		}
	}
	{
		v := make(map[interface{}]interface{})
		if json.Unmarshal(data, &v) == nil {
			score++
		}
	}
	return score
}
