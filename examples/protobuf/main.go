package protobuf

import (
	"bytes"
	"fmt"
	"reflect"

	pb "github.com/dvyukov/go-fuzz/examples/protobuf/pb"
	"github.com/golang/protobuf/proto"
)

func Fuzz(data []byte) int {
	return fuzz(data, false)
}

func FuzzText(data []byte) int {
	return fuzz(data, true)
}

func fuzz(data []byte, text bool) int {
	ctors := []func() proto.Message{
		func() proto.Message { return new(pb.M0) },
		func() proto.Message { return new(pb.M1) },
		func() proto.Message { return new(pb.M2) },
		func() proto.Message { return new(pb.M3) },
		func() proto.Message { return new(pb.M4) },
		func() proto.Message { return new(pb.M5) },
		func() proto.Message { return new(pb.M6) },
		func() proto.Message { return new(pb.M7) },
		func() proto.Message { return new(pb.M8) },
		func() proto.Message { return new(pb.M9) },
		func() proto.Message { return new(pb.M10) },
		func() proto.Message { return new(pb.M11) },
		func() proto.Message { return new(pb.M12) },
		func() proto.Message { return new(pb.M13) },
		func() proto.Message { return new(pb.M14) },
		func() proto.Message { return new(pb.M15) },
		func() proto.Message { return new(pb.M16) },
		func() proto.Message { return new(pb.M17) },
		func() proto.Message { return new(pb.M18) },
		func() proto.Message { return new(pb.M19) },
		func() proto.Message { return new(pb.M20) },
		func() proto.Message { return new(pb.M21) },
		func() proto.Message { return new(pb.M22) },
		func() proto.Message { return new(pb.M23) },
		func() proto.Message { return new(pb.M24) },
		func() proto.Message { return new(pb.M25) },
	}
	datas := ""
	if text {
		datas = string(data)
	}
	score := 0
	for _, ctor := range ctors {
		v := ctor()
		var err error
		if text {
			err = proto.UnmarshalText(datas, v)
		} else {
			err = proto.Unmarshal(data, v)
		}
		if err != nil {
			continue
		}
		score = 1
		var data1 []byte
		if text {
			var buf bytes.Buffer
			err = proto.MarshalText(&buf, v)
			data1 = buf.Bytes()
		} else {
			data1, err = proto.Marshal(v)
		}
		if err != nil {
			panic(err)
		}
		v1 := ctor()
		if text {
			err = proto.UnmarshalText(string(data1), v1)
		} else {
			err = proto.Unmarshal(data1, v1)
		}
		if err != nil {
			panic(err)
		}
		if !reflect.DeepEqual(v, v1) {
			// These types contain floats, NaNs don't compare equal.
			if _, ok := v.(*pb.M10); ok {
				continue
			}
			if _, ok := v.(*pb.M11); ok {
				continue
			}
			// M18 contains extensions map which can either nil or empty, does not compare equal.
			if _, ok := v.(*pb.M18); ok {
				continue
			}
			fmt.Printf("v0: %#v\n", v)
			fmt.Printf("v1: %#v\n", v1)
			panic(fmt.Sprintf("non idempotent marshal of %T", v))
		}
	}
	return score
}
