package ui

import (
	"fmt"
	"testing"

	"github.com/dobrevit/nft-tui/internal/model"
)

// makeSyntheticRuleset returns a Ruleset shaped like a busy edge box:
// chains×rulesPerChain rules across one inet filter table. Every rule
// has a unique source address, port, and comment so the filter has
// realistic work to do (no shared substrings to short-circuit).
func makeSyntheticRuleset(chains, rulesPerChain int) *model.Ruleset {
	tbl := &model.Table{Family: "inet", Name: "filter"}
	for ci := 0; ci < chains; ci++ {
		ch := &model.Chain{Table: tbl, Name: fmt.Sprintf("chain%d", ci), IsBase: false}
		for ri := 0; ri < rulesPerChain; ri++ {
			h := uint64(ci*rulesPerChain + ri + 1)
			saddr := fmt.Sprintf("10.%d.%d.%d", (h>>16)&0xff, (h>>8)&0xff, h&0xff)
			dport := fmt.Sprintf("%d", 1024+(int(h)%60000))
			r := &model.Rule{
				Chain:   ch,
				Handle:  h,
				IIfName: fmt.Sprintf("eth%d", h%4),
				SAddr:   saddr,
				DPort:   dport,
				Verdict: "accept",
				Comment: fmt.Sprintf("synthetic rule %d", h),
				NFT: fmt.Sprintf(
					`iifname "eth%d" ip saddr %s tcp dport %s counter accept comment "synthetic rule %d"`,
					h%4, saddr, dport, h),
			}
			r.RebuildSearchKey()
			ch.Rules = append(ch.Rules, r)
		}
		tbl.Chains = append(tbl.Chains, ch)
	}
	return &model.Ruleset{Tables: []*model.Table{tbl}}
}

func BenchmarkRuleMatches_Substring(b *testing.B) {
	r := &model.Rule{
		NFT:     `iifname "eth0" ip saddr 10.42.17.5 tcp dport 5432 accept comment "internal pg"`,
		Comment: "internal pg",
		IIfName: "eth0",
		SAddr:   "10.42.17.5",
		DPort:   "5432",
		Verdict: "accept",
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ruleMatches(r, "5432")
	}
}

func BenchmarkSearchRuleset_10k_hits(b *testing.B) {
	rs := makeSyntheticRuleset(50, 200) // 10,000 rules
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// "10.0." matches a small subset early — exercises both the match
		// and the cap.
		_ = searchRuleset(rs, "10.0.", 500)
	}
}

func BenchmarkSearchRuleset_10k_miss(b *testing.B) {
	// Worst case: a query no rule matches forces a full scan.
	rs := makeSyntheticRuleset(50, 200)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = searchRuleset(rs, "no-such-substring-anywhere", 500)
	}
}

func BenchmarkSearchRuleset_50k_miss(b *testing.B) {
	rs := makeSyntheticRuleset(100, 500) // 50,000 rules
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = searchRuleset(rs, "no-such-substring-anywhere", 500)
	}
}
