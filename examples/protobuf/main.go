package protobuf

import (
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
	vars := []proto.Message{
		new(pb.M0),
		new(pb.M1),
		new(pb.M2),
		new(pb.M3),
		new(pb.M4),
		new(pb.M5),
		new(pb.M6),
		new(pb.M7),
		new(pb.M8),
		new(pb.M9),
		new(pb.M10),
		new(pb.M11),
		new(pb.M12),
		new(pb.M13),
		new(pb.M14),
		new(pb.M15),
		new(pb.M16),
		new(pb.M17),
		new(pb.M18),
		new(pb.M19),
		new(pb.M20),
		new(pb.M21),
		new(pb.M22),
		new(pb.M23),
		new(pb.M24),
		new(pb.M25),
	}
	datas := ""
	if text {
		datas = string(data)
	}
	score := 0
	for _, v := range vars {
		var err error
		if text {
			err = proto.UnmarshalText(datas, v)
		} else {
			err = proto.Unmarshal(data, v)
		}
		if err == nil {
			score++
			if _, err := proto.Marshal(v); err != nil {
				panic(err)
			}
		}
	}
	return score
}
