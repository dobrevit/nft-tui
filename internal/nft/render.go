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

	"github.com/dobrevit/nft-tui/internal/model"
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

// Stable identifiers for the contents of a netlink register, used as
// the discriminator on regInfo.kind. Keep these in sync with the
// metaKeyInfo / payloadInfo / ctKeyInfo constructors below.
const (
	kindMetaIIFName  = "meta-iifname"
	kindMetaOIFName  = "meta-oifname"
	kindMetaL4Proto  = "meta-l4proto"
	kindMetaNFProto  = "meta-nfproto"
	kindCTState      = "ct-state"
	kindIPSAddr      = "ip-saddr"
	kindIPDAddr      = "ip-daddr"
	kindIP6SAddr     = "ip6-saddr"
	kindIP6DAddr     = "ip6-daddr"
	kindTransportSrc = "transport-sport"
	kindTransportDst = "transport-dport"
)

// regInfo describes the meaning of a value currently held in a netlink
// register. The renderer fills this in on "load" expressions (Meta, Payload,
// Ct, Bitwise) and consumes it on Cmp/Lookup.
type regInfo struct {
	// kind is a short identifier like kindMetaIIFName, kindIPSAddr, kindCTState.
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
//
// sets is the per-table set index; used to inline anonymous-set lookups
// (`@__set0`) as `{ a, b, c }`. Pass a nil or empty map if no inlining is
// desired.
// ruleRenderer is the working state for one rule's translation. Each
// expression type has its own handle* method; RenderRule is the
// dispatch loop. Splitting per-expr keeps any one function small and
// keeps the SonarLint cognitive-complexity score down.
type ruleRenderer struct {
	out  RuleRendering
	regs map[uint32]regInfo
	// matches: "left op right" fragments from the match phase.
	matches []string
	// actions: counter/log/verdict/reject fragments.
	actions []string
	// transport tracks the L4 proto context (set by the meta-l4proto
	// cmp; consumed by transport-{sport,dport} matches and lookups).
	transport string
	// nfproto-guard elision state. nft hides the implicit `meta
	// nfproto ipv4|ipv6` guard before an ip-family payload match in
	// an inet table. We defer the emit and drop it if the next match
	// is the matching ip family.
	pendingNFProto    string
	pendingNFProtoFam string
	// sets is the per-table set index for anonymous-set inlining.
	sets map[string]*model.Set
}

func RenderRule(r *nftables.Rule, sets map[string]*model.Set) RuleRendering {
	rr := &ruleRenderer{
		regs: map[uint32]regInfo{},
		sets: sets,
	}
	for _, e := range r.Exprs {
		rr.handleExpr(e)
	}
	return rr.finalize()
}

// finalize flushes any deferred state (the nfproto guard) and joins
// matches + actions into the final nft text.
func (r *ruleRenderer) finalize() RuleRendering {
	if r.pendingNFProto != "" {
		r.matches = append(r.matches, r.pendingNFProto)
	}
	parts := append([]string{}, r.matches...)
	parts = append(parts, r.actions...)
	r.out.NFT = strings.Join(parts, " ")
	return r.out
}

// emitMatch appends text to matches. If a pending nfproto guard is
// queued, it is either elided (when text is a matching ip-family
// match) or flushed first.
func (r *ruleRenderer) emitMatch(text string) {
	if r.pendingNFProto != "" {
		elide := (r.pendingNFProtoFam == "ipv4" && strings.HasPrefix(text, "ip ")) ||
			(r.pendingNFProtoFam == "ipv6" && strings.HasPrefix(text, "ip6 "))
		if !elide {
			r.matches = append(r.matches, r.pendingNFProto)
		}
		r.pendingNFProto = ""
		r.pendingNFProtoFam = ""
	}
	r.matches = append(r.matches, text)
}

// handleExpr dispatches one expression to its specific handler.
func (r *ruleRenderer) handleExpr(e expr.Any) {
	switch x := e.(type) {
	case *expr.Meta:
		r.regs[x.Register] = metaKeyInfo(x.Key)
	case *expr.Payload:
		r.regs[x.DestRegister] = payloadInfo(x.Base, x.Offset, x.Len)
	case *expr.Ct:
		r.regs[x.Register] = ctKeyInfo(x.Key)
	case *expr.Bitwise:
		r.handleBitwise(x)
	case *expr.Cmp:
		r.handleCmp(x)
	case *expr.Lookup:
		r.handleLookup(x)
	case *expr.Counter:
		r.handleCounter(x)
	case *expr.Log:
		r.actions = append(r.actions, renderLog(x))
	case *expr.Verdict:
		v := renderVerdict(x)
		r.out.Verdict = v
		r.actions = append(r.actions, v)
	case *expr.Immediate:
		// Immediate sets a register or carries a verdict; verdicts are
		// handled separately. We surface unknown immediates so the
		// operator can drop to raw mode.
		r.actions = append(r.actions, "<expr:immediate>")
	case *expr.Reject:
		r.actions = append(r.actions, renderReject(x))
	case *expr.Limit:
		r.actions = append(r.actions, renderLimit(x))
	case *expr.Quota:
		r.actions = append(r.actions, fmt.Sprintf("quota %d bytes used %d", x.Bytes, x.Consumed))
	case *expr.NAT:
		r.actions = append(r.actions, renderNAT(x))
	case *expr.Masq:
		r.actions = append(r.actions, "masquerade")
	case *expr.Redir:
		r.actions = append(r.actions, "redirect")
	case *expr.Match:
		r.emitMatch(fmt.Sprintf("<xt-match:%s>", x.Name))
	case *expr.Target:
		r.actions = append(r.actions, fmt.Sprintf("<xt-target:%s>", x.Name))
	default:
		r.actions = append(r.actions, fmt.Sprintf("<expr:%T>", e))
	}
}

// handleBitwise records a mask applied to a previously-loaded register,
// used by Cmp to render `ip saddr 10.0.0.0/24` and `ct state {…}`.
func (r *ruleRenderer) handleBitwise(x *expr.Bitwise) {
	if src, ok := r.regs[x.SourceRegister]; ok {
		src.mask = append([]byte(nil), x.Mask...)
		r.regs[x.DestRegister] = src
		return
	}
	r.regs[x.DestRegister] = regInfo{kind: "bitwise", label: "<expr:bitwise>"}
}

// handleCmp emits a match line (or, for special patterns, mutates
// renderer state without emitting).
func (r *ruleRenderer) handleCmp(x *expr.Cmp) {
	info, ok := r.regs[x.Register]
	if !ok {
		r.emitMatch(fmt.Sprintf("<expr:cmp reg=%d>", x.Register))
		return
	}
	if r.cmpCTStateMask(info, x) {
		return
	}
	if r.cmpL4Proto(info, x) {
		return
	}
	if r.cmpNFProto(info, x) {
		return
	}
	r.cmpGeneric(info, x)
}

// cmpCTStateMask handles the `ct state <names>` encoding: ct-state
// load → bitwise(mask) → cmp(neq, 0). Returns true if it matched.
func (r *ruleRenderer) cmpCTStateMask(info regInfo, x *expr.Cmp) bool {
	if info.kind != kindCTState || info.mask == nil ||
		x.Op != expr.CmpOpNeq || !allZero(x.Data) {
		return false
	}
	names := formatCTState(binary.LittleEndian.Uint32(info.mask))
	r.out.CTState = names
	r.emitMatch("ct state " + names)
	return true
}

// cmpL4Proto absorbs a `meta l4proto = N` cmp into the transport
// context. nft elides this match entirely in its textual output.
func (r *ruleRenderer) cmpL4Proto(info regInfo, x *expr.Cmp) bool {
	if info.kind != kindMetaL4Proto || x.Op != expr.CmpOpEq || len(x.Data) != 1 {
		return false
	}
	r.transport = l4protoName(x.Data[0])
	return true
}

// cmpNFProto absorbs a `meta nfproto = ipv4|ipv6` cmp into the
// pending-guard state. emitMatch decides later whether to drop it.
func (r *ruleRenderer) cmpNFProto(info regInfo, x *expr.Cmp) bool {
	if info.kind != kindMetaNFProto || x.Op != expr.CmpOpEq || len(x.Data) != 1 {
		return false
	}
	r.pendingNFProtoFam = nfprotoName(x.Data[0])
	r.pendingNFProto = "meta nfproto " + r.pendingNFProtoFam
	return true
}

// cmpGeneric is the fallback for "real" Cmp matches. Emits "lhs op rhs"
// and captures decoded fields into the columnar-view fields of out.
func (r *ruleRenderer) cmpGeneric(info regInfo, x *expr.Cmp) {
	lhs := info.label
	rhs := formatCmpData(info, x.Data)
	op := cmpOp(x.Op)

	switch info.kind {
	case kindIPSAddr, kindIP6SAddr, kindIPDAddr, kindIP6DAddr:
		captureAddr(&r.out, info.kind, rhs)
	case kindMetaIIFName:
		r.out.IIfName = strings.Trim(rhs, `"`)
	case kindMetaOIFName:
		r.out.OIfName = strings.Trim(rhs, `"`)
	case kindCTState:
		r.out.CTState = rhs
	}

	if info.kind == kindTransportSrc || info.kind == kindTransportDst {
		lhs = transportLabel(info.kind, r.transport)
		if info.kind == kindTransportSrc {
			r.out.SPort = rhs
		} else {
			r.out.DPort = rhs
		}
		if r.transport != "" {
			r.out.Proto = r.transport
		}
	}

	if op == "" {
		r.emitMatch(fmt.Sprintf("%s %s", lhs, rhs))
	} else {
		r.emitMatch(fmt.Sprintf("%s %s %s", lhs, op, rhs))
	}
}

// handleLookup renders a set / map lookup, inlining anonymous sets
// from the per-table set index when available.
func (r *ruleRenderer) handleLookup(x *expr.Lookup) {
	label := r.lookupLabel(x.SourceRegister)
	op, setRef := r.lookupRefAndOp(x)
	switch {
	case x.Invert:
		r.emitMatch(fmt.Sprintf("%s != %s", label, setRef))
	case op != "":
		r.emitMatch(fmt.Sprintf("%s %s %s", label, op, setRef))
		if op == "vmap" {
			// The map's value supplies the rule's verdict; surface
			// that in the decoded Verdict field for the columnar
			// view, lacking a more specific signal.
			r.out.Verdict = "vmap " + x.SetName
		}
	default:
		r.emitMatch(fmt.Sprintf("%s %s", label, setRef))
	}
}

// lookupLabel renders the LHS of a lookup, applying transport-protocol
// context to transport-port loads.
func (r *ruleRenderer) lookupLabel(srcReg uint32) string {
	info, ok := r.regs[srcReg]
	if !ok {
		return "<reg>"
	}
	if info.kind == kindTransportSrc || info.kind == kindTransportDst {
		if r.transport != "" {
			r.out.Proto = r.transport
		}
		return transportLabel(info.kind, r.transport)
	}
	return info.label
}

// lookupRefAndOp returns the verb (`vmap` / `map` / `""`) and the
// set/map reference (either `@name` or an inlined `{ … }` literal).
func (r *ruleRenderer) lookupRefAndOp(x *expr.Lookup) (op, setRef string) {
	setRef = "@" + x.SetName
	s, found := r.sets[x.SetName]
	if !found {
		return "", setRef
	}
	switch {
	case s.IsMap && s.ValueIsVerdict:
		op = "vmap"
	case s.IsMap:
		op = "map"
	}
	if isAnonymousSet(x.SetName) {
		if elems := renderSetElements(s); len(elems) > 0 {
			setRef = "{ " + strings.Join(elems, ", ") + " }"
		}
	}
	return op, setRef
}

// handleCounter records counter values and emits the canonical
// `counter packets N bytes M` form.
func (r *ruleRenderer) handleCounter(x *expr.Counter) {
	r.out.HasCounter = true
	r.out.CounterPackets = x.Packets
	r.out.CounterBytes = x.Bytes
	r.actions = append(r.actions,
		fmt.Sprintf("counter packets %d bytes %d", x.Packets, x.Bytes))
}

// captureAddr stashes the rendered address into the appropriate decoded field.
func captureAddr(out *RuleRendering, kind, rhs string) {
	switch kind {
	case kindIPSAddr, kindIP6SAddr:
		out.SAddr = rhs
	case kindIPDAddr, kindIP6DAddr:
		out.DAddr = rhs
	}
}

// --- Meta -------------------------------------------------------------------

func metaKeyInfo(k expr.MetaKey) regInfo {
	switch k {
	case expr.MetaKeyIIFNAME:
		return regInfo{kind: kindMetaIIFName, label: "iifname"}
	case expr.MetaKeyOIFNAME:
		return regInfo{kind: kindMetaOIFName, label: "oifname"}
	case expr.MetaKeyIIF:
		return regInfo{kind: "meta-iif", label: "iif"}
	case expr.MetaKeyOIF:
		return regInfo{kind: "meta-oif", label: "oif"}
	case expr.MetaKeyMARK:
		return regInfo{kind: "meta-mark", label: "meta mark"}
	case expr.MetaKeyL4PROTO:
		return regInfo{kind: kindMetaL4Proto, label: "meta l4proto"}
	case expr.MetaKeyNFPROTO:
		return regInfo{kind: kindMetaNFProto, label: "meta nfproto"}
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
			return regInfo{kind: kindIPSAddr, label: "ip saddr", prefixBits: int(length) * 8}
		case offset == 16 && length >= 1 && length <= 4:
			return regInfo{kind: kindIPDAddr, label: "ip daddr", prefixBits: int(length) * 8}
		case offset == 8 && length >= 1 && length <= 16:
			return regInfo{kind: kindIP6SAddr, label: "ip6 saddr", prefixBits: int(length) * 8}
		case offset == 24 && length >= 1 && length <= 16:
			return regInfo{kind: kindIP6DAddr, label: "ip6 daddr", prefixBits: int(length) * 8}
		case offset == 9 && length == 1:
			return regInfo{kind: "ip-protocol", label: "ip protocol"}
		case offset == 6 && length == 1:
			return regInfo{kind: "ip6-nexthdr", label: "ip6 nexthdr"}
		}
		return regInfo{kind: "payload-network", label: fmt.Sprintf("@nh,%d,%d", offset, length)}

	case expr.PayloadBaseTransportHeader:
		switch {
		case offset == 0 && length == 2:
			return regInfo{kind: kindTransportSrc, label: kindTransportSrc}
		case offset == 2 && length == 2:
			return regInfo{kind: kindTransportDst, label: kindTransportDst}
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
		return regInfo{kind: kindCTState, label: "ct state"}
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
	case kindIPSAddr, kindIPDAddr:
		return formatIPv4Prefix(data, info.mask, info.prefixBits)
	case kindIP6SAddr, kindIP6DAddr:
		return formatIPv6Prefix(data, info.mask, info.prefixBits)
	case kindTransportSrc, kindTransportDst:
		if len(data) == 2 {
			return fmt.Sprintf("%d", binary.BigEndian.Uint16(data))
		}
	case kindMetaIIFName, kindMetaOIFName:
		// NUL-padded IFNAMSIZ buffer.
		s := strings.TrimRight(string(data), "\x00")
		return `"` + s + `"`
	case kindMetaL4Proto, "ip-protocol", "ip6-nexthdr":
		if len(data) == 1 {
			return l4protoName(data[0])
		}
	case kindMetaNFProto:
		if len(data) == 1 {
			return nfprotoName(data[0])
		}
	case kindCTState:
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

// transportLabel turns (kindTransportDst, "tcp") into "tcp dport". Falls
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

// isAnonymousSet recognises the names the kernel auto-assigns to anonymous
// sets/maps generated from inline literals like `{ 22, 80, 443 }`.
func isAnonymousSet(name string) bool {
	return strings.HasPrefix(name, "__set") || strings.HasPrefix(name, "__map")
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
