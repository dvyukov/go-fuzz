package gopacket

import (
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
)

func FuzzDNS(data []byte) int {
	gopacket.NewPacket(data, layers.LayerTypeDNS, gopacket.Default)
	return 0
}
