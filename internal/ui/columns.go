package ui

import (
	"fmt"
	"strings"

	"github.com/dobrevit/nft-tui/internal/model"
)

// columnSpec describes one column of the rule list table.
// value is invoked per row to produce the cell text.
type columnSpec struct {
	header string
	value  func(r *model.Rule) string
}

// columnSet pairs a human-readable name with its column list.
type columnSet struct {
	name    string
	columns []columnSpec
}

// columnPresets are the named column layouts, in display order.
// The first is the runtime default. `c` cycles through them; the
// --columns CLI flag selects one explicitly at startup.
func columnPresets() []*columnSet {
	return []*columnSet{
		{
			name:    "default",
			columns: []columnSpec{colHandle, colPPS, colBytes, colIIF, colOIF, colSrc, colDst, colDPort, colVerdict, colRule},
		},
		{
			name:    "minimal",
			columns: []columnSpec{colHandle, colPPS, colRule},
		},
		{
			name:    "debug",
			columns: []columnSpec{colHandle, colPPS, colBytes, colIIF, colOIF, colProto, colSrc, colSPort, colDst, colDPort, colCTState, colVerdict, colRule},
		},
		{
			name:    "wide",
			columns: []columnSpec{colHandle, colPPS, colBytes, colVerdict, colRule},
		},
	}
}

// ColumnPresetNames returns the preset names comma-separated, used by
// the --columns flag help and the error path on unknown names.
func ColumnPresetNames() string {
	ps := columnPresets()
	names := make([]string, len(ps))
	for i, p := range ps {
		names[i] = p.name
	}
	return strings.Join(names, ", ")
}

// LookupColumnPreset returns the index of the named preset, or -1 if
// the name is unknown. Empty string returns 0 (the default).
func LookupColumnPreset(name string) int {
	if name == "" {
		return 0
	}
	for i, p := range columnPresets() {
		if strings.EqualFold(p.name, name) {
			return i
		}
	}
	return -1
}

// --- column definitions ----------------------------------------------------

var (
	colHandle = columnSpec{
		header: "H#",
		value:  func(r *model.Rule) string { return fmt.Sprintf("%d", r.Handle) },
	}
	colPPS = columnSpec{
		header: "PKTS",
		value:  func(r *model.Rule) string { return humanCount(r.Counter.Packets) },
	}
	colBytes = columnSpec{
		header: "BYTES",
		value:  func(r *model.Rule) string { return humanBytes(r.Counter.Bytes) },
	}
	colIIF = columnSpec{
		header: "IIF",
		value:  func(r *model.Rule) string { return blankDash(r.IIfName) },
	}
	colOIF = columnSpec{
		header: "OIF",
		value:  func(r *model.Rule) string { return blankDash(r.OIfName) },
	}
	colProto = columnSpec{
		header: "PROTO",
		value:  func(r *model.Rule) string { return blankDash(r.Proto) },
	}
	colSrc = columnSpec{
		header: "SRC",
		value:  func(r *model.Rule) string { return blankDash(r.SAddr) },
	}
	colDst = columnSpec{
		header: "DST",
		value:  func(r *model.Rule) string { return blankDash(r.DAddr) },
	}
	colSPort = columnSpec{
		header: "SPORT",
		value:  func(r *model.Rule) string { return blankDash(r.SPort) },
	}
	colDPort = columnSpec{
		header: "DPORT",
		value:  func(r *model.Rule) string { return blankDash(r.DPort) },
	}
	colCTState = columnSpec{
		header: "CT-STATE",
		value:  func(r *model.Rule) string { return blankDash(r.CTState) },
	}
	colVerdict = columnSpec{
		header: "VERDICT",
		value:  func(r *model.Rule) string { return blankDash(r.Verdict) },
	}
	colRule = columnSpec{
		header: "RULE",
		value:  func(r *model.Rule) string { return truncate(r.NFT, 80) },
	}
)
