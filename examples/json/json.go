package json

import (
	"encoding/json"
	"reflect"
)

func Fuzz(data []byte) int {
	score := 0
	for _, ctor := range []func() interface{}{
		func() interface{} { return nil },
		func() interface{} { return []interface{}{} },
		func() interface{} { return map[interface{}]interface{}{} },
		func() interface{} { return map[string]interface{}{} },
		func() interface{} { return new(S) },
	} {
		v := ctor()
		if json.Unmarshal(data, v) != nil {
			continue
		}
		score = 1
		if s, ok := v.(*S); ok {
			if len(s.P) == 0 {
				s.P = []byte(`""`)
			}
		}
		data1, err := json.Marshal(v)
		if err != nil {
			panic(err)
		}
		v1 := ctor()
		if json.Unmarshal(data1, v1) != nil {
			continue
		}
		if s, ok := v.(*S); ok {
			// Some additional escaping happens with P.
			s.P = nil
			v1.(*S).P = nil
		}
		if !reflect.DeepEqual(v, v1) {
			panic("non-idempotent marshal")
		}
	}
	return score
}

type S struct {
	A int    `json:",omitempty"`
	B string `json:"B1,omitempty"`
	C float64
	D bool
	E uint8
	F []byte
	G interface{}
	H map[string]interface{}
	I map[string]string
	J []interface{}
	K []string
	L S1
	M *S1
	N *int
	O **int
	P json.RawMessage
	Q Marshaller
	R int `json:"-"`
	S int `json:",string"`
}

type S1 struct {
	A int
	B string
}

type Marshaller struct {
	v string
}

func (m *Marshaller) MarshalJSON() ([]byte, error) {
	return json.Marshal(m.v)
}

func (m *Marshaller) UnmarshalJSON(data []byte) error {
	return json.Unmarshal(data, &m.v)
}
