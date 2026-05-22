// Package nft is the adapter between the kernel (via google/nftables netlink)
// and our internal model.
//
// render.go owns the binary-AST → nft-text rendering. The kernel exposes a
// rule as a sequence of `expr.Any` instructions operating on a small register
// file; we fold that sequence into an nft-syntax statement.
//
// We deliberately recognise only the cases we can render faithfully. Any
// expression we don't fully understand is emitted as a `<expr:Type>` token so
// the operator can see the rule is more complex than the form-friendly view
// suggests and drop to raw mode.
package nft

import (
	"encoding/binary"
	"fmt"
	"net"
	"net/netip"
	"strings"

	"github.com/google/nftables"
	"github.com/google/nftables/expr"
)

// RuleRendering is the output of rendering one rule.
type RuleRendering struct {
	// NFT is the canonical nft-syntax representation.
	NFT string

	// The decoded "well-known" fields, populated when the renderer
	// recognises them. These power the columnar list view.
	IIfName string
	OIfName string
	SAddr   string
	DAddr   string
	SPort   string
	DPort   string
	Proto   string
	CTState string
	Verdict string

	// CounterPackets / CounterBytes are non-zero only if the rule carries
	// a `counter` statement.
	CounterPackets uint64
	CounterBytes   uint64
	HasCounter     bool
}

// regInfo describes the meaning of a value currently held in a netlink
// register. The renderer fills this in on "load" expressions (Meta, Payload,
// Ct, Bitwise) and consumes it on Cmp/Lookup.
type regInfo struct {
	// kind is a short identifier like "meta-iifname", "ip-saddr", "ct-state".
	kind string
	// label is the nft-text fragment used on the left of a comparison,
	// e.g. `iifname`, `ip saddr`, `tcp dport`, `ct state`.
	label string
	// mask, if non-nil, was applied to this register by a preceding Bitwise
	// expression. Used to turn `ip saddr == 10.0.0.0 mask 255.255.255.0`
	// into `ip saddr 10.0.0.0/24`.
	mask []byte
	// prefixBits is set for IPv4/IPv6 payload loads of length < address
	// width. nft optimises e.g. `ip saddr 10.0.0.0/24` into a 3-byte
	// payload load + 3-byte cmp; this field records the implicit prefix.
	prefixBits int
}

// RenderRule walks the expression list and produces an nft-text statement
// plus decoded fields. It never returns an error: anything it cannot decode
// is surfaced as a `<expr:...>` token in the rendered text.
func RenderRule(r *nftables.Rule) RuleRendering {
	var (
		out  RuleRendering
		regs = map[uint32]regInfo{}
		// matches collects "left op right" fragments from the match phase.
		matches []string
		// actions collects counter/log/verdict/reject fragments emitted
		// during the rule.
		actions []string
	)

	// Most rules read transport-layer ports only after a prior cmp on the
	// L4 proto byte set the implicit protocol context. Track it so we can
	// label tcp/udp ports correctly.
	transport := ""

	for _, e := range r.Exprs {
		switch x := e.(type) {

		case *expr.Meta:
			info := metaKeyInfo(x.Key)
			regs[x.Register] = info
			if info.kind == "meta-l4proto" {
				// nothing to emit yet; the cmp will do it
			}

		case *expr.Payload:
			info := payloadInfo(x.Base, x.Offset, x.Len)
			regs[x.DestRegister] = info

		case *expr.Ct:
			info := ctKeyInfo(x.Key)
			regs[x.Register] = info

		case *expr.Bitwise:
			// Bitwise is typically used to mask a previously-loaded value
			// before a Cmp, e.g. ip saddr -> AND netmask -> cmp network.
			if src, ok := regs[x.SourceRegister]; ok {
				src.mask = append([]byte(nil), x.Mask...)
				regs[x.DestRegister] = src
			} else {
				regs[x.DestRegister] = regInfo{kind: "bitwise", label: "<expr:bitwise>"}
			}

		case *expr.Cmp:
			info, ok := regs[x.Register]
			if !ok {
				matches = append(matches, fmt.Sprintf("<expr:cmp reg=%d>", x.Register))
				continue
			}

			// Special pattern: `ct state <names>` is emitted by the kernel
			// as `ct state` → bitwise(AND mask) → cmp(neq, 0). The mask
			// itself encodes which states are being matched.
			if info.kind == "ct-state" && info.mask != nil &&
				x.Op == expr.CmpOpNeq && allZero(x.Data) {
				names := formatCTState(binary.LittleEndian.Uint32(info.mask))
				out.CTState = names
				matches = append(matches, "ct state "+names)
				continue
			}

			lhs := info.label
			rhs := formatCmpData(info, x.Data)
			op := cmpOp(x.Op)

			// L4 proto cmp sets transport context for subsequent port loads.
			if info.kind == "meta-l4proto" && op == "" && len(x.Data) == 1 {
				transport = l4protoName(x.Data[0])
				continue
			}

			// Capture decoded fields into the rendering result.
			switch info.kind {
			case "ip-saddr", "ip6-saddr", "ip-daddr", "ip6-daddr":
				captureAddr(&out, info.kind, rhs)
			case "meta-iifname":
				out.IIfName = strings.Trim(rhs, `"`)
			case "meta-oifname":
				out.OIfName = strings.Trim(rhs, `"`)
			case "ct-state":
				out.CTState = rhs
			}

			if info.kind == "transport-sport" || info.kind == "transport-dport" {
				lhs = transportLabel(info.kind, transport)
				if info.kind == "transport-sport" {
					out.SPort = rhs
				} else {
					out.DPort = rhs
				}
				if transport != "" {
					out.Proto = transport
				}
			}

			if op == "" {
				matches = append(matches, fmt.Sprintf("%s %s", lhs, rhs))
			} else {
				matches = append(matches, fmt.Sprintf("%s %s %s", lhs, op, rhs))
			}

		case *expr.Lookup:
			info, ok := regs[x.SourceRegister]
			label := "<reg>"
			if ok {
				label = info.label
				if info.kind == "transport-sport" || info.kind == "transport-dport" {
					label = transportLabel(info.kind, transport)
					if transport != "" {
						out.Proto = transport
					}
				}
			}
			setRef := "@" + x.SetName
			// nft hides the auto-generated `__set%d` names for anonymous
			// sets, but we don't have the element list here. Surface them
			// with a leading marker so the operator can tell.
			if strings.HasPrefix(x.SetName, "__set") {
				setRef = "@" + x.SetName + " (anonymous)"
			}
			if x.Invert {
				matches = append(matches, fmt.Sprintf("%s != %s", label, setRef))
			} else {
				matches = append(matches, fmt.Sprintf("%s %s", label, setRef))
			}

		case *expr.Counter:
			out.HasCounter = true
			out.CounterPackets = x.Packets
			out.CounterBytes = x.Bytes
			actions = append(actions, fmt.Sprintf("counter packets %d bytes %d", x.Packets, x.Bytes))

		case *expr.Log:
			actions = append(actions, renderLog(x))

		case *expr.Verdict:
			v := renderVerdict(x)
			out.Verdict = v
			actions = append(actions, v)

		case *expr.Immediate:
			// Immediate is used both for verdicts (via the Verdict field)
			// and for setting register values (e.g. for NAT). Verdict is
			// already handled as a separate type; here we only emit a
			// placeholder if we see one we don't model.
			actions = append(actions, "<expr:immediate>")

		case *expr.Reject:
			actions = append(actions, renderReject(x))

		case *expr.Limit:
			actions = append(actions, renderLimit(x))

		case *expr.Quota:
			actions = append(actions, fmt.Sprintf("quota %d bytes used %d", x.Bytes, x.Consumed))

		case *expr.NAT:
			actions = append(actions, renderNAT(x))

		case *expr.Masq:
			actions = append(actions, "masquerade")

		case *expr.Redir:
			actions = append(actions, "redirect")

		case *expr.Match:
			matches = append(matches, fmt.Sprintf("<xt-match:%s>", x.Name))

		case *expr.Target:
			actions = append(actions, fmt.Sprintf("<xt-target:%s>", x.Name))

		default:
			actions = append(actions, fmt.Sprintf("<expr:%T>", e))
		}
	}

	parts := append([]string{}, matches...)
	parts = append(parts, actions...)
	out.NFT = strings.Join(parts, " ")
	if out.Verdict == "" {
		// nft prints a bare `continue` rule by leaving the verdict empty,
		// but most rules carry one. If there's no verdict at all, that's
		// fine — many rules are pure logging/counting statements.
	}
	return out
}

// captureAddr stashes the rendered address into the appropriate decoded field.
func captureAddr(out *RuleRendering, kind, rhs string) {
	switch kind {
	case "ip-saddr", "ip6-saddr":
		out.SAddr = rhs
	case "ip-daddr", "ip6-daddr":
		out.DAddr = rhs
	}
}

// --- Meta -------------------------------------------------------------------

func metaKeyInfo(k expr.MetaKey) regInfo {
	switch k {
	case expr.MetaKeyIIFNAME:
		return regInfo{kind: "meta-iifname", label: "iifname"}
	case expr.MetaKeyOIFNAME:
		return regInfo{kind: "meta-oifname", label: "oifname"}
	case expr.MetaKeyIIF:
		return regInfo{kind: "meta-iif", label: "iif"}
	case expr.MetaKeyOIF:
		return regInfo{kind: "meta-oif", label: "oif"}
	case expr.MetaKeyMARK:
		return regInfo{kind: "meta-mark", label: "meta mark"}
	case expr.MetaKeyL4PROTO:
		return regInfo{kind: "meta-l4proto", label: "meta l4proto"}
	case expr.MetaKeyNFPROTO:
		return regInfo{kind: "meta-nfproto", label: "meta nfproto"}
	case expr.MetaKeyPROTOCOL:
		return regInfo{kind: "meta-protocol", label: "meta protocol"}
	case expr.MetaKeyPRIORITY:
		return regInfo{kind: "meta-priority", label: "meta priority"}
	case expr.MetaKeySKUID:
		return regInfo{kind: "meta-skuid", label: "meta skuid"}
	case expr.MetaKeySKGID:
		return regInfo{kind: "meta-skgid", label: "meta skgid"}
	}
	return regInfo{kind: "meta-unknown", label: fmt.Sprintf("meta <key=%d>", k)}
}

// --- Payload ----------------------------------------------------------------

func payloadInfo(base expr.PayloadBase, offset, length uint32) regInfo {
	switch base {
	case expr.PayloadBaseNetworkHeader:
		// We can't tell from the offset alone whether the L3 header is IPv4
		// or IPv6; the family of the table constrains it, but we don't have
		// that here. Best-effort by (offset, length) pairs.
		//
		// IPv4 prefix-match optimisation: nft emits `ip saddr 10.0.0.0/24`
		// as a 3-byte payload load + 3-byte cmp (and similar for shorter
		// prefixes), saving one byte over the masked /24 form. We treat any
		// load at offset 12/16 of length 1..4 as an IPv4 saddr/daddr with
		// an implicit prefix of (length*8) bits.
		switch {
		case offset == 12 && length >= 1 && length <= 4:
			return regInfo{kind: "ip-saddr", label: "ip saddr", prefixBits: int(length) * 8}
		case offset == 16 && length >= 1 && length <= 4:
			return regInfo{kind: "ip-daddr", label: "ip daddr", prefixBits: int(length) * 8}
		case offset == 8 && length >= 1 && length <= 16:
			return regInfo{kind: "ip6-saddr", label: "ip6 saddr", prefixBits: int(length) * 8}
		case offset == 24 && length >= 1 && length <= 16:
			return regInfo{kind: "ip6-daddr", label: "ip6 daddr", prefixBits: int(length) * 8}
		case offset == 9 && length == 1:
			return regInfo{kind: "ip-protocol", label: "ip protocol"}
		case offset == 6 && length == 1:
			return regInfo{kind: "ip6-nexthdr", label: "ip6 nexthdr"}
		}
		return regInfo{kind: "payload-network", label: fmt.Sprintf("@nh,%d,%d", offset, length)}

	case expr.PayloadBaseTransportHeader:
		switch {
		case offset == 0 && length == 2:
			return regInfo{kind: "transport-sport", label: "transport-sport"}
		case offset == 2 && length == 2:
			return regInfo{kind: "transport-dport", label: "transport-dport"}
		}
		return regInfo{kind: "payload-transport", label: fmt.Sprintf("@th,%d,%d", offset, length)}

	case expr.PayloadBaseLLHeader:
		return regInfo{kind: "payload-link", label: fmt.Sprintf("@ll,%d,%d", offset, length)}
	}
	return regInfo{kind: "payload-unknown", label: fmt.Sprintf("@?,%d,%d", offset, length)}
}

// --- Ct ---------------------------------------------------------------------

func ctKeyInfo(k expr.CtKey) regInfo {
	switch k {
	case expr.CtKeySTATE:
		return regInfo{kind: "ct-state", label: "ct state"}
	case expr.CtKeyMARK:
		return regInfo{kind: "ct-mark", label: "ct mark"}
	case expr.CtKeySTATUS:
		return regInfo{kind: "ct-status", label: "ct status"}
	case expr.CtKeyPROTOCOL:
		return regInfo{kind: "ct-protocol", label: "ct protocol"}
	}
	return regInfo{kind: "ct-unknown", label: fmt.Sprintf("ct <key=%d>", k)}
}

// --- Cmp helpers ------------------------------------------------------------

func cmpOp(op expr.CmpOp) string {
	switch op {
	case expr.CmpOpEq:
		return "" // nft elides `==`; `ip saddr 10.0.0.0` not `ip saddr == 10.0.0.0`
	case expr.CmpOpNeq:
		return "!="
	case expr.CmpOpLt:
		return "<"
	case expr.CmpOpLte:
		return "<="
	case expr.CmpOpGt:
		return ">"
	case expr.CmpOpGte:
		return ">="
	}
	return fmt.Sprintf("<cmpop:%d>", op)
}

func formatCmpData(info regInfo, data []byte) string {
	switch info.kind {
	case "ip-saddr", "ip-daddr":
		return formatIPv4Prefix(data, info.mask, info.prefixBits)
	case "ip6-saddr", "ip6-daddr":
		return formatIPv6Prefix(data, info.mask, info.prefixBits)
	case "transport-sport", "transport-dport":
		if len(data) == 2 {
			return fmt.Sprintf("%d", binary.BigEndian.Uint16(data))
		}
	case "meta-iifname", "meta-oifname":
		// NUL-padded IFNAMSIZ buffer.
		s := strings.TrimRight(string(data), "\x00")
		return `"` + s + `"`
	case "meta-l4proto", "ip-protocol", "ip6-nexthdr":
		if len(data) == 1 {
			return l4protoName(data[0])
		}
	case "meta-nfproto":
		if len(data) == 1 {
			return nfprotoName(data[0])
		}
	case "ct-state":
		if len(data) == 4 {
			return formatCTState(binary.LittleEndian.Uint32(data))
		}
	}
	return formatBytesHex(data)
}

// nfprotoName renders an NFPROTO_* family number to its nft name. Used
// when an inet table emits an explicit family guard ahead of a payload
// match.
func nfprotoName(n byte) string {
	switch n {
	case 2:
		return "ipv4"
	case 7:
		return "bridge"
	case 10:
		return "ipv6"
	}
	return fmt.Sprintf("%d", n)
}

// formatIPv4Prefix renders an IPv4 address, taking either an explicit
// netmask or a load-truncated prefix length into account.
func formatIPv4Prefix(data, mask []byte, prefixBits int) string {
	// Right-pad short loads to a full 4 bytes; nft emits e.g. 3 data bytes
	// for a /24, and we want to display as 10.0.0.0/24 not 0x0a0000.
	full := make([]byte, 4)
	copy(full, data)
	ip := net.IPv4(full[0], full[1], full[2], full[3]).To4()

	switch {
	case len(mask) == 4:
		ones, bits := net.IPMask(mask).Size()
		if bits == 32 && ones < 32 {
			return fmt.Sprintf("%s/%d", ip.String(), ones)
		}
	case len(data) > 0 && len(data) < 4:
		return fmt.Sprintf("%s/%d", ip.String(), len(data)*8)
	case prefixBits > 0 && prefixBits < 32:
		return fmt.Sprintf("%s/%d", ip.String(), prefixBits)
	}
	return ip.String()
}

// formatIPv6Prefix is the IPv6 counterpart to formatIPv4Prefix.
func formatIPv6Prefix(data, mask []byte, prefixBits int) string {
	var arr [16]byte
	copy(arr[:], data)
	ip := netip.AddrFrom16(arr)

	switch {
	case len(mask) == 16:
		ones, bits := net.IPMask(mask).Size()
		if bits == 128 && ones < 128 {
			return fmt.Sprintf("%s/%d", ip.String(), ones)
		}
	case len(data) > 0 && len(data) < 16:
		return fmt.Sprintf("%s/%d", ip.String(), len(data)*8)
	case prefixBits > 0 && prefixBits < 128:
		return fmt.Sprintf("%s/%d", ip.String(), prefixBits)
	}
	return ip.String()
}

// transportLabel turns ("transport-dport", "tcp") into "tcp dport". Falls
// back to a generic "transport dport" when the L4 proto is unknown.
func transportLabel(kind, proto string) string {
	field := strings.TrimPrefix(kind, "transport-")
	if proto == "" {
		return "transport " + field
	}
	return proto + " " + field
}

// allZero reports whether every byte in data is zero. Used to recognise the
// `ct state` mask pattern (cmp != 0).
func allZero(data []byte) bool {
	for _, b := range data {
		if b != 0 {
			return false
		}
	}
	return true
}

func formatBytesHex(b []byte) string {
	var sb strings.Builder
	sb.WriteString("0x")
	for _, c := range b {
		fmt.Fprintf(&sb, "%02x", c)
	}
	return sb.String()
}

// l4protoName maps an IP protocol number to its name.
func l4protoName(n byte) string {
	switch n {
	case 1:
		return "icmp"
	case 6:
		return "tcp"
	case 17:
		return "udp"
	case 47:
		return "gre"
	case 50:
		return "esp"
	case 51:
		return "ah"
	case 58:
		return "icmpv6"
	case 132:
		return "sctp"
	case 136:
		return "udplite"
	}
	return fmt.Sprintf("%d", n)
}

// formatCTState renders the nf_conntrack state bitmask. The bits are the same
// as the kernel's NF_CT_STATE_* constants << 0 (NEW=1<<0 ESTABLISHED=1<<1 ...).
func formatCTState(mask uint32) string {
	bits := []struct {
		v uint32
		n string
	}{
		{1 << 0, "invalid"},     // NF_CT_STATE_INVALID_BIT is actually 0; nft maps bit 0 to "invalid"
		{1 << 1, "established"}, // NEW=1, ESTABLISHED=2
		{1 << 2, "related"},
		{1 << 3, "new"},
		{1 << 4, "untracked"},
	}
	// nft's bit assignments (see include/linux/netfilter/nf_conntrack_common.h):
	// IP_CT_NEW=0 -> bit 0 == "new"? But nft uses a different mapping. Use
	// the values from libnftnl: invalid=0x1, established=0x2, related=0x4,
	// new=0x8, untracked=0x40. Re-do explicitly:
	bits = []struct {
		v uint32
		n string
	}{
		{0x01, "invalid"},
		{0x02, "established"},
		{0x04, "related"},
		{0x08, "new"},
		{0x40, "untracked"},
	}
	var names []string
	for _, b := range bits {
		if mask&b.v != 0 {
			names = append(names, b.n)
		}
	}
	if len(names) == 0 {
		return fmt.Sprintf("0x%x", mask)
	}
	if len(names) == 1 {
		return names[0]
	}
	return "{ " + strings.Join(names, ", ") + " }"
}

// --- Action renderers ------------------------------------------------------

func renderVerdict(v *expr.Verdict) string {
	switch v.Kind {
	case expr.VerdictAccept:
		return "accept"
	case expr.VerdictDrop:
		return "drop"
	case expr.VerdictReturn:
		return "return"
	case expr.VerdictJump:
		return "jump " + v.Chain
	case expr.VerdictGoto:
		return "goto " + v.Chain
	case expr.VerdictContinue:
		return "continue"
	case expr.VerdictQueue:
		return "queue"
	case expr.VerdictStop:
		return "stop"
	}
	return fmt.Sprintf("<verdict:%d>", v.Kind)
}

func renderLog(l *expr.Log) string {
	var b strings.Builder
	b.WriteString("log")
	if len(l.Data) > 0 {
		prefix := strings.TrimRight(string(l.Data), "\x00")
		fmt.Fprintf(&b, ` prefix "%s"`, prefix)
	}
	return b.String()
}

func renderReject(r *expr.Reject) string {
	switch r.Type {
	case 0: // NFT_REJECT_ICMP_UNREACH
		return fmt.Sprintf("reject with icmp type %d", r.Code)
	case 1: // NFT_REJECT_TCP_RST
		return "reject with tcp reset"
	case 2: // NFT_REJECT_ICMPX_UNREACH (inet family — portable)
		return fmt.Sprintf("reject with icmpx type %d", r.Code)
	}
	return "reject"
}

func renderLimit(l *expr.Limit) string {
	unit := "second"
	switch l.Unit {
	case 0:
		unit = "second"
	case 1:
		unit = "minute"
	case 2:
		unit = "hour"
	case 3:
		unit = "day"
	case 4:
		unit = "week"
	}
	return fmt.Sprintf("limit rate %d/%s", l.Rate, unit)
}

func renderNAT(n *expr.NAT) string {
	verb := "nat"
	switch n.Type {
	case expr.NATTypeSourceNAT:
		verb = "snat"
	case expr.NATTypeDestNAT:
		verb = "dnat"
	}
	return verb
}
