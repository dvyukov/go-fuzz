package protobuf

import (
	pb "github.com/dvyukov/go-fuzz/examples/protobuf/pb"
	"github.com/golang/protobuf/proto"
)

func Fuzz(data []byte) int {
	defer func() {
		v := recover()
		if v != nil {
			str := ""
			switch vv := v.(type) {
			case string:
				str = vv
			case error:
				str = vv.Error()
			}
			if str == "reflect: call of reflect.Value.SetMapIndex on zero Value" {
				return
			}
			panic(v)
		}
	}()
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
	score := 0
	for _, v := range vars {
		if err := proto.Unmarshal(data, v); err == nil {
			score++
			if _, err := proto.Marshal(v); err != nil {
				panic(err)
			}
		}
	}
	return score
}
