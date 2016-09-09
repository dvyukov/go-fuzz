// Copyright 2015 Dmitry Vyukov. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

// automatically generated, do not modify

package mygame

import (
	flatbuffers "github.com/google/flatbuffers/go"
)

type Weapon struct {
	_tab flatbuffers.Table
}

func (rcv *Weapon) Init(buf []byte, i flatbuffers.UOffsetT) {
	rcv._tab.Bytes = buf
	rcv._tab.Pos = i
}

func (rcv *Weapon) Name() string {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(4))
	if o != 0 {
		return rcv._tab.String(o + rcv._tab.Pos)
	}
	return ""
}

func (rcv *Weapon) X() float32 {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(6))
	if o != 0 {
		return rcv._tab.GetFloat32(o + rcv._tab.Pos)
	}
	return 0
}

func WeaponStart(builder *flatbuffers.Builder) { builder.StartObject(2) }
func WeaponAddName(builder *flatbuffers.Builder, name flatbuffers.UOffsetT) {
	builder.PrependUOffsetTSlot(0, flatbuffers.UOffsetT(name), 0)
}
func WeaponAddX(builder *flatbuffers.Builder, x float32)          { builder.PrependFloat32Slot(1, x, 0) }
func WeaponEnd(builder *flatbuffers.Builder) flatbuffers.UOffsetT { return builder.EndObject() }
