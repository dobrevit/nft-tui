package ui

import (
	"strings"
	"testing"

	"github.com/dobrevit/nft-tui/internal/model"
)

func TestLookupColumnPreset(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"default", 0},
		{"DEFAULT", 0},
		{"", 0},
		{"minimal", 1},
		{"debug", 2},
		{"wide", 3},
		{"chartreuse", -1},
	}
	for _, c := range cases {
		if got := LookupColumnPreset(c.in); got != c.want {
			t.Errorf("LookupColumnPreset(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestColumnPresetNamesContainsAll(t *testing.T) {
	got := ColumnPresetNames()
	for _, p := range columnPresets() {
		if !strings.Contains(got, p.name) {
			t.Errorf("ColumnPresetNames() = %q missing %q", got, p.name)
		}
	}
}

func TestColumnValuesNonNil(t *testing.T) {
	// A rule with every decoded field set so each column has data.
	r := &model.Rule{
		Handle:  7,
		NFT:     `iifname "eth0" tcp dport 22 accept`,
		IIfName: "eth0", OIfName: "eth1",
		SAddr: "10.0.0.0/24", DAddr: "10.0.1.0/24",
		SPort: "1024", DPort: "22",
		Proto: "tcp", CTState: "established", Verdict: "accept",
		Counter: model.Counter{Present: true, Packets: 100, Bytes: 5000},
	}
	for _, p := range columnPresets() {
		for _, spec := range p.columns {
			if spec.header == "" {
				t.Errorf("preset %s: empty header", p.name)
			}
			if v := spec.value(r); v == "" {
				t.Errorf("preset %s, column %s: empty value for fully-populated rule", p.name, spec.header)
			}
		}
	}
}
