package ui

import (
	"testing"

	"github.com/dobrevit/nft-tui/internal/model"
)

func TestRuleMatches(t *testing.T) {
	r := &model.Rule{
		Handle:  42,
		NFT:     `iifname "eth0" ip saddr 10.0.0.0/24 tcp dport 5432 accept`,
		Comment: "internal postgres",
		IIfName: "eth0",
		SAddr:   "10.0.0.0/24",
		DPort:   "5432",
		Verdict: "accept",
	}

	cases := []struct {
		query string
		want  bool
	}{
		{"", true},             // empty query matches everything
		{"42", true},           // handle
		{"eth0", true},         // iif
		{"ETH0", true},         // case-insensitive
		{"5432", true},         // dport
		{"postgres", true},     // comment
		{"10.0.0", true},       // saddr substring
		{"accept", true},       // verdict
		{"drop", false},        // not present
		{"eth1", false},        // wrong interface
		{"reject", false},      // not present
		{"established", false}, // not present
	}
	for _, c := range cases {
		got := ruleMatches(r, c.query)
		if got != c.want {
			t.Errorf("ruleMatches(%q) = %v, want %v", c.query, got, c.want)
		}
	}
}

func TestSearchRuleset(t *testing.T) {
	tbl := &model.Table{Family: "inet", Name: "filter"}
	ch := &model.Chain{Table: tbl, Name: "input"}
	r1 := &model.Rule{Chain: ch, Handle: 1, NFT: `iifname "eth0" accept`, IIfName: "eth0", Verdict: "accept"}
	r2 := &model.Rule{Chain: ch, Handle: 2, NFT: `tcp dport 22 accept`, DPort: "22", Verdict: "accept"}
	r3 := &model.Rule{Chain: ch, Handle: 3, NFT: `counter drop`, Verdict: "drop"}
	ch.Rules = []*model.Rule{r1, r2, r3}
	tbl.Chains = []*model.Chain{ch}

	rs := &model.Ruleset{Tables: []*model.Table{tbl}}

	hits := searchRuleset(rs, "accept", 100)
	if len(hits) != 2 {
		t.Fatalf("got %d hits, want 2", len(hits))
	}
	if hits[0].rule.Handle != 1 || hits[1].rule.Handle != 2 {
		t.Errorf("hit handles = %d,%d; want 1,2", hits[0].rule.Handle, hits[1].rule.Handle)
	}

	// maxResults cap
	hits = searchRuleset(rs, "accept", 1)
	if len(hits) != 1 {
		t.Errorf("got %d hits with cap=1, want 1", len(hits))
	}

	// empty query returns nothing (we don't want to flood the UI on `:` with no input)
	if got := searchRuleset(rs, "", 100); got != nil {
		t.Errorf("empty query returned %d hits; want nil", len(got))
	}
}
