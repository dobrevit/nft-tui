package ui

import (
	"strings"
	"testing"

	"github.com/dobrevit/nft-tui/internal/model"
	"github.com/dobrevit/nft-tui/internal/staged"
)

func TestRenderNFTScript(t *testing.T) {
	if got := renderNFTScript(nil); !strings.Contains(got, "empty") {
		t.Errorf("empty ops should render an `# (empty)` marker; got %q", got)
	}

	ops := []staged.Op{
		&staged.AddRule{
			Family: model.FamilyINet, Table: "filter", Chain: "input",
			Body: "tcp dport 22 accept",
		},
		&staged.DeleteRule{
			Family: model.FamilyINet, Table: "filter", Chain: "input",
			Handle: 11,
		},
		&staged.ReplaceRule{
			Family: model.FamilyINet, Table: "filter", Chain: "forward",
			Handle: 5, Body: "ip saddr 10.0.0.0/24 accept",
		},
	}
	got := renderNFTScript(ops)
	want := "" +
		"add rule inet filter input tcp dport 22 accept\n" +
		"delete rule inet filter input handle 11\n" +
		"replace rule inet filter forward handle 5 ip saddr 10.0.0.0/24 accept\n"
	if got != want {
		t.Errorf("\n  got:\n%s\n  want:\n%s", got, want)
	}
}
