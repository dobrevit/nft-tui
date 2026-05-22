package nft

import (
	"encoding/binary"
	"fmt"
	"net"
	"net/netip"

	"github.com/google/nftables"

	"github.com/dobrevit/nft-tui/internal/model"
)

// formatSetElement renders one element key of a set, based on the set's
// declared key type. Falls back to hex for types we don't decode yet.
//
// nftables set keys are kernel-byte-order (network for addrs/ports). The
// length validation guards against malformed elements without panicking.
func formatSetElement(keyType string, key []byte) string {
	switch keyType {
	case "ipv4_addr":
		if len(key) == 4 {
			return net.IPv4(key[0], key[1], key[2], key[3]).To4().String()
		}
	case "ipv6_addr":
		if len(key) == 16 {
			var a [16]byte
			copy(a[:], key)
			return netip.AddrFrom16(a).String()
		}
	case "inet_service":
		if len(key) == 2 {
			return fmt.Sprintf("%d", binary.BigEndian.Uint16(key))
		}
	case "inet_proto":
		if len(key) == 1 {
			return l4protoName(key[0])
		}
	case "ether_addr":
		if len(key) == 6 {
			return net.HardwareAddr(key).String()
		}
	case "mark":
		if len(key) == 4 {
			return fmt.Sprintf("0x%x", binary.BigEndian.Uint32(key))
		}
	}
	return formatBytesHex(key)
}

// renderSetElements returns the elements of a set as a slice of
// already-formatted strings. IntervalEnd sentinels (the upper bound of
// interval sets) are paired with the previous element as "low-high".
func renderSetElements(s *model.Set) []string {
	out := make([]string, 0, len(s.Elements))
	var pending string
	for _, el := range s.Elements {
		if el.IntervalEnd {
			if pending != "" {
				out = append(out, pending+"-"+el.Key)
				pending = ""
			} else {
				out = append(out, "-"+el.Key)
			}
			continue
		}
		if pending != "" {
			out = append(out, pending)
		}
		pending = el.Key
	}
	if pending != "" {
		out = append(out, pending)
	}
	return out
}

// convertSetElement turns a netlink-decoded SetElement into our model.
func convertSetElement(keyType string, e nftables.SetElement) model.SetElement {
	return model.SetElement{
		Key:          formatSetElement(keyType, e.Key),
		IntervalEnd:  e.IntervalEnd,
		Comment:      e.Comment,
		TimeoutLeft:  e.Expires,
	}
}
