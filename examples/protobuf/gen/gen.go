// Copyright 2015 Dmitry Vyukov. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

package main

import (
	"fmt"
	pb "github.com/dvyukov/go-fuzz/examples/protobuf/pb"
	"github.com/golang/protobuf/proto"
	"os"
)

func main() {
	vars := []proto.Message{
		&pb.M0{F: proto.Int32(0)},
		&pb.M0{F: proto.Int32(-1000000), XXX_unrecognized: []byte{0, 1, 2}},
		&pb.M1{F: proto.Int64(0)},
		&pb.M1{F: proto.Int64(100)},
		&pb.M1{F: proto.Int64(123123123123123123)},
		&pb.M1{F: proto.Int64(-100)},
		&pb.M1{F: proto.Int64(-123123123123123123)},
		&pb.M2{},
		&pb.M2{F: proto.Uint32(123123)},
		&pb.M3{},
		&pb.M3{F: proto.Uint64(123123123123123123)},
		&pb.M4{F: proto.Int32(123123)},
		&pb.M5{F: proto.Int64(123123)},
		&pb.M5{F: proto.Int64(-123123)},
		&pb.M6{XXX_unrecognized: []byte{0, 1, 2}},
		&pb.M6{F: proto.Uint32(123123), XXX_unrecognized: []byte{0, 1, 2}},
		&pb.M7{F: proto.Uint64(123123123123)},
		&pb.M8{F: proto.Int32(-123123)},
		&pb.M9{F: proto.Int64(-123123123123)},
		&pb.M10{F: proto.Float64(123123.123123)},
		&pb.M11{F: proto.Float32(123123.123123)},
		&pb.M12{F: proto.Bool(true)},
		&pb.M13{},
		&pb.M13{F: proto.String("")},
		&pb.M13{F: proto.String("foo")},
		&pb.M13{F: proto.String("&pb.M6{F: proto.Uint32(123123), XXX_unrecognized: []byte{0,1,2}},")},
		&pb.M13{F: proto.String("\x00\x01\x02")},
		&pb.M14{},
		&pb.M14{F: []byte{0, 1, 2}},
		&pb.M14{F: []byte("&pb.M6{F: proto.Uint32(123123), XXX_unrecognized: []byte{0,1,2}},")},
		&pb.M15{F0: proto.Int32(123)},
		&pb.M15{F0: proto.Int32(123), F1: proto.String("foo"), F2: []byte{1, 2, 3}, F4: proto.Bool(false)},
		&pb.M16{},
		&pb.M16{F: pb.Corpus_UNIVERSAL.Enum()},
		&pb.M16{F: pb.Corpus_PRODUCTS.Enum()},
		&pb.M17{F: &pb.M15{F0: proto.Int32(123)}},
		&pb.M17{F: &pb.M15{F0: proto.Int32(123), F1: proto.String("foo"), F2: []byte{1, 2, 3}, F4: proto.Bool(false)}},
		func() proto.Message {
			v := &pb.M18{F0: proto.String("foo")}
			proto.SetExtension(v, pb.E_F1, 42)
			return v
		}(),
		&pb.M19{},
		&pb.M19{F: []int32{0, -123, 500, 123123123}},
		&pb.M20{F: []string{"", "foo", "\x00\x01\x02"}},
		&pb.M21{F: []*pb.M15{&pb.M15{F0: proto.Int32(123)}, &pb.M15{F0: proto.Int32(123), F1: proto.String("foo"), F2: []byte{1, 2, 3}, F4: proto.Bool(false)}}},
		&pb.M22{F: []*pb.M2{&pb.M2{}}},
		&pb.M22{F: []*pb.M2{&pb.M2{}, &pb.M2{F: proto.Uint32(123123)}}},
		&pb.M23{},
		&pb.M23{F: map[int32]string{42: "", 11: "foo", 123123123: "\x00\x01\x02"}},
		&pb.M24{F: map[string]*pb.M2{"": &pb.M2{}, "foo": &pb.M2{}, "\x00\x01\x02": &pb.M2{F: proto.Uint32(123123)}}},
		&pb.M25{},
		&pb.M25{F0: proto.String("")},
		&pb.M25{F0: proto.String("foo")},
		&pb.M25{F1: &pb.M2{}},
		&pb.M25{F1: &pb.M2{F: proto.Uint32(123123)}},
		&pb.M25{F2: pb.Corpus_UNIVERSAL.Enum()},
	}
	for i, v := range vars {
		if false {
			data, err := proto.Marshal(v)
			if err != nil {
				panic(err)
			}
			f, err := os.Create(fmt.Sprintf("/tmp/proto/%v", i))
			if err != nil {
				panic(err)
			}
			f.Write(data)
			f.Close()
		} else {
			f, err := os.Create(fmt.Sprintf("/tmp/proto/%v", i))
			if err != nil {
				panic(err)
			}
			fmt.Printf("%v: %+v\n", i, v)
			err = proto.MarshalText(f, v)
			if err != nil {
				panic(err)
			}
			f.Close()
		}
	}
}
