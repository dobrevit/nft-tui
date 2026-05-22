// Package model holds the in-memory representation of an nftables ruleset.
//
// The shape mirrors the kernel object hierarchy (Ruleset → Table → Chain →
// Rule, plus Sets/Maps/FlowTables/Objects under Table). Rules carry both a
// structured Expr list (decoded from netlink via google/nftables) and a
// pre-rendered nft-syntax string for display.
package model

import (
	"fmt"
	"strings"
	"time"
)

type Family string

const (
	FamilyIP     Family = "ip"
	FamilyIP6    Family = "ip6"
	FamilyINet   Family = "inet"
	FamilyARP    Family = "arp"
	FamilyBridge Family = "bridge"
	FamilyNetdev Family = "netdev"
)

type Ruleset struct {
	Tables    []*Table
	FetchedAt time.Time
}

type Table struct {
	Family     Family
	Name       string
	Chains     []*Chain
	Sets       []*Set
	FlowTables []*FlowTable
	Counters   []*NamedCounter
	Comment    string
}

type Chain struct {
	Table *Table `json:"-"`
	Name  string

	// Base-chain fields. For regular chains all are zero values and IsBase is false.
	IsBase   bool
	Type     string // "filter" | "nat" | "route"
	Hook     string // "input" | "output" | "forward" | "prerouting" | "postrouting" | "ingress" | "egress"
	Priority int32
	Policy   string // "accept" | "drop"
	Device   string // netdev/ingress

	Rules   []*Rule
	Comment string
}

type Rule struct {
	Chain   *Chain `json:"-"`
	Handle  uint64
	Comment string

	// NFT is the canonical nft-syntax form of the rule, produced by our
	// renderer. It is the single source of truth for display.
	NFT string

	// Decoded fields used by the columnar rule list view. Zero-value means
	// "unknown / not present in this rule".
	IIfName string
	OIfName string
	SAddr   string
	DAddr   string
	SPort   string
	DPort   string
	Proto   string // "tcp" | "udp" | "icmp" | "icmpv6" | ""
	CTState string
	Verdict string // "accept" | "drop" | "reject" | "jump X" | ...

	Counter Counter

	// SearchKey is a precomputed lower-cased concatenation of every field
	// the filter looks at. Built once at read time (call RebuildSearchKey
	// after constructing a Rule) so per-keystroke substring filtering is
	// allocation-free. Not for display.
	SearchKey string `json:"-"`
}

// RebuildSearchKey recomputes r.SearchKey from r's current field values.
// Call after constructing or modifying a Rule so the filter / search
// path sees fresh content.
func (r *Rule) RebuildSearchKey() {
	var b strings.Builder
	b.Grow(len(r.NFT) + 64)
	fmt.Fprintf(&b, "%d", r.Handle)
	for _, s := range [...]string{
		r.NFT, r.Comment,
		r.IIfName, r.OIfName,
		r.SAddr, r.DAddr,
		r.SPort, r.DPort,
		r.Proto, r.CTState, r.Verdict,
	} {
		b.WriteByte(0)
		b.WriteString(s)
	}
	r.SearchKey = strings.ToLower(b.String())
}

type Counter struct {
	Packets uint64
	Bytes   uint64
	Present bool // whether this rule actually carries a counter
}

type Set struct {
	Table    *Table `json:"-"`
	Name     string
	KeyType  string
	Flags    SetFlags
	Timeout  time.Duration
	Elements []SetElement
	Size     int
	Comment  string
}

type SetFlags struct {
	Constant bool
	Dynamic  bool
	Interval bool
	Timeout  bool
	Counter  bool
}

type SetElement struct {
	Key string
	// IntervalEnd is true when this element is the sentinel upper bound of
	// an interval-set entry (the lower bound is the previous element).
	IntervalEnd bool
	// TimeoutLeft is the remaining lifetime of a dynamic-set element; zero
	// if the set has no timeout or the element is permanent.
	TimeoutLeft time.Duration
	Counter     Counter
	Comment     string
}

type FlowTable struct {
	Table    *Table `json:"-"`
	Name     string
	Hook     string
	Priority int32
	Devices  []string
}

type NamedCounter struct {
	Table   *Table `json:"-"`
	Name    string
	Counter Counter
}
