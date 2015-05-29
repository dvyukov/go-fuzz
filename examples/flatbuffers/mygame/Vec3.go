// automatically generated, do not modify

package mygame

import (
	flatbuffers "github.com/google/flatbuffers/go"
)
type Vec3 struct {
	_tab flatbuffers.Struct
}

func (rcv *Vec3) Init(buf []byte, i flatbuffers.UOffsetT) {
	rcv._tab.Bytes = buf
	rcv._tab.Pos = i
}

func (rcv *Vec3) X() float32 { return rcv._tab.GetFloat32(rcv._tab.Pos + flatbuffers.UOffsetT(0)) }
func (rcv *Vec3) Y() float32 { return rcv._tab.GetFloat32(rcv._tab.Pos + flatbuffers.UOffsetT(4)) }
func (rcv *Vec3) Z() float32 { return rcv._tab.GetFloat32(rcv._tab.Pos + flatbuffers.UOffsetT(8)) }

func CreateVec3(builder *flatbuffers.Builder, x float32, y float32, z float32) flatbuffers.UOffsetT {
    builder.Prep(4, 12)
    builder.PrependFloat32(z)
    builder.PrependFloat32(y)
    builder.PrependFloat32(x)
    return builder.Offset()
}
