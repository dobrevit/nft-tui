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
// already-formatted strings. For plain sets IntervalEnd sentinels (the
// upper bound of interval sets) are paired with the previous element as
// "low-high". For maps each element is rendered as "key : value".
func renderSetElements(s *model.Set) []string {
	if s.IsMap {
		return renderMapElements(s)
	}
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

// renderMapElements emits "key : value" pairs for map elements. Interval
// sentinels in maps are rare (interval-keyed maps); we drop them on the
// Phase 2 floor for now and surface a TODO marker so the operator knows.
func renderMapElements(s *model.Set) []string {
	out := make([]string, 0, len(s.Elements))
	for _, el := range s.Elements {
		if el.IntervalEnd {
			continue
		}
		if el.Value == "" {
			out = append(out, el.Key+" : <missing-value>")
		} else {
			out = append(out, el.Key+" : "+el.Value)
		}
	}
	return out
}

// convertSetElement turns a netlink-decoded SetElement into our model. The
// containing Set's metadata (IsMap / DataType / ValueIsVerdict) is needed
// to render the value side of map elements.
func convertSetElement(s *model.Set, e nftables.SetElement) model.SetElement {
	me := model.SetElement{
		Key:         formatSetElement(s.KeyType, e.Key),
		IntervalEnd: e.IntervalEnd,
		Comment:     e.Comment,
		TimeoutLeft: e.Expires,
	}
	if !s.IsMap {
		return me
	}
	switch {
	case s.ValueIsVerdict && e.VerdictData != nil:
		me.Value = renderVerdict(e.VerdictData)
	case len(e.Val) > 0:
		me.Value = formatSetElement(s.DataType, e.Val)
	}
	return me
}
