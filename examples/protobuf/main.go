// Copyright 2015 Dmitry Vyukov. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

package protobuf

import (
	"bytes"
	"fmt"
	"strings"

	. "github.com/dvyukov/go-fuzz/examples/fuzz"
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
		if !DeepEqual(v, v1) {
			fmt.Printf("v0: %#v\n", v)
			fmt.Printf("v1: %#v\n", v1)
			panic(fmt.Sprintf("non idempotent marshal of %T", v))
		}
		if text {
			// TODO: Marshal/Unmarshal to binary.
		} else {
			var buf bytes.Buffer
			err := proto.MarshalText(&buf, v)
			if err != nil {
				fmt.Printf("failed to MarshalText: %#v\n", v)
				panic(err)
			}
			v2 := ctor()
			err = proto.UnmarshalText(buf.String(), v2)
			if err != nil {
				if strings.Contains(err.Error(), "unexpected byte 0x2f") {
					continue // known bug
				}
				fmt.Printf("failed to UnmarshalText: %q\n", buf.Bytes())
				panic(err)
			}
			if !DeepEqual(v, v2) {
				fmt.Printf("v0: %#v\n", v)
				fmt.Printf("v2: %#v\n", v2)
				panic(fmt.Sprintf("non idempotent text marshal of %T", v))
			}
		}
	}
	return score
}
