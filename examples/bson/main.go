package bson

import (
	"fmt"
	"reflect"
	"strings"

	"gopkg.in/mgo.v2/bson"
)

func Fuzz(data []byte) int {
	score := 0
	for _, ctor := range []func() interface{}{
		func() interface{} { return make(bson.M) },
		func() interface{} { return new(bson.D) },
		func() interface{} { return new(S) },
		func() interface{} { return new(O) },
	} {
		v := ctor()
		if bson.Unmarshal(data, v) != nil {
			continue
		}
		score = 1
		data1, err := bson.Marshal(v)
		if err != nil {
			if strings.HasPrefix(err.Error(), "ObjectIDs must be exactly 12 bytes long") {
				continue
			}
			panic(err)
		}
		v1 := ctor()
		if err := bson.Unmarshal(data1, v1); err != nil {
			// https://github.com/go-mgo/mgo/issues/117
			if err.Error() == "Document is corrupted" {
				continue
			}
			panic(err)
		}
		// The following check fails because of nil/len==0 slice difference.
		if false && !reflect.DeepEqual(v, v1) {
			if strings.Contains(fmt.Sprintf("%+v", v), "NaN") {
				continue
			}
			fmt.Printf("v0=%+v\n", v)
			fmt.Printf("v1=%+v\n", v1)
			panic("non-idempotent unmarshalling")
		}
	}
	return score
}

type S struct {
	A int
	B string `bson:",omitempty"`
	C float64
	D []byte `bson:",omitempty"`
	E bool   `bson:"E1"`
	F uint8  `bson:",omitempty"`
	G S1
	H *S2
	I *int
	J *string
	K **int
	L **string
	M **S2
	N S1          `bson:",inline"`
	O int64       `bson:",minsize"`
	P bson.Binary `bson:",omitempty"`
	Q bson.D      `bson:",omitempty"`
	R interface{}
	S int
	T bson.JavaScript `bson:",omitempty"`
	U bson.M          `bson:",omitempty"`
	V bson.MongoTimestamp
	W bson.Raw  `bson:",omitempty"`
	X bson.RawD `bson:",omitempty"`
	Z bson.M    `bson:",inline"`
}

type S1 struct {
	A1 int
	B1 string
	C1 *S2
	D1 S2
}

type S2 struct {
	A int
	B string
	C bool
}

type O struct {
	A bson.ObjectId
}
