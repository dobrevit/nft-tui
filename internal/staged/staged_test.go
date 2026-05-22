package staged

import (
	"strings"
	"testing"

	"github.com/dobrevit/nft-tui/internal/model"
)

func TestOpNFT(t *testing.T) {
	cases := []struct {
		name string
		op   Op
		want string
	}{
		{
			name: "add rule with comment",
			op: &AddRule{
				Family: model.FamilyINet, Table: "filter", Chain: "input",
				Body:    `tcp dport 22 counter accept`,
				Comment: "ssh",
			},
			want: `add rule inet filter input tcp dport 22 counter accept comment "ssh"`,
		},
		{
			name: "add rule without comment",
			op: &AddRule{
				Family: model.FamilyIP, Table: "nat", Chain: "postrouting",
				Body: `oifname "eth0" masquerade`,
			},
			want: `add rule ip nat postrouting oifname "eth0" masquerade`,
		},
		{
			name: "insert before handle",
			op: &InsertRule{
				Family: model.FamilyINet, Table: "filter", Chain: "input",
				Position: 17, Body: `ip saddr 10.0.0.0/24 accept`,
			},
			want: `insert rule inet filter input position 17 ip saddr 10.0.0.0/24 accept`,
		},
		{
			name: "insert after handle (add rule ... position H)",
			op: &InsertRule{
				Family: model.FamilyINet, Table: "filter", Chain: "input",
				Position: 17, After: true, Body: `ip saddr 10.0.0.0/24 accept`,
			},
			want: `add rule inet filter input position 17 ip saddr 10.0.0.0/24 accept`,
		},
		{
			name: "delete rule by handle",
			op: &DeleteRule{
				Family: model.FamilyINet, Table: "filter", Chain: "input",
				Handle: 42,
			},
			want: `delete rule inet filter input handle 42`,
		},
		{
			name: "replace rule",
			op: &ReplaceRule{
				Family: model.FamilyINet, Table: "filter", Chain: "input",
				Handle: 7, Body: `ip saddr 10.0.0.0/24 accept`,
				Comment: "internal LAN",
			},
			want: `replace rule inet filter input handle 7 ip saddr 10.0.0.0/24 accept comment "internal LAN"`,
		},
		{
			name: "flush chain",
			op: &FlushChain{
				Family: model.FamilyINet, Table: "filter", Chain: "input",
			},
			want: `flush chain inet filter input`,
		},
		{
			// nftables has no escape mechanism inside `comment "X"`,
			// so an embedded " can't survive — substitute with '.
			// Round-trip integration test
			// (TestIntegration_StagedOpsRoundTrip) confirms `nft -c`
			// accepts the result.
			name: "comment with embedded double quote substituted",
			op: &AddRule{
				Family: model.FamilyINet, Table: "filter", Chain: "input",
				Body: `accept`, Comment: `says "hi"`,
			},
			want: `add rule inet filter input accept comment "says 'hi'"`,
		},
		{
			name: "comment with newline strips control chars",
			op: &AddRule{
				Family: model.FamilyINet, Table: "filter", Chain: "input",
				Body: `accept`, Comment: "line one\nline two",
			},
			want: `add rule inet filter input accept comment "line oneline two"`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.op.NFT(); got != c.want {
				t.Errorf("\n  got:  %q\n  want: %q", got, c.want)
			}
		})
	}
}

func TestChangeListSerialization(t *testing.T) {
	var cl ChangeList
	if cl.NFT() != "" {
		t.Errorf("empty ChangeList.NFT() = %q, want empty", cl.NFT())
	}

	cl.Append(&AddRule{
		Family: model.FamilyINet, Table: "filter", Chain: "input",
		Body: "tcp dport 22 accept",
	})
	cl.Append(&DeleteRule{
		Family: model.FamilyINet, Table: "filter", Chain: "input",
		Handle: 11,
	})
	if cl.Len() != 2 {
		t.Errorf("Len() = %d, want 2", cl.Len())
	}

	got := cl.NFT()
	want := "add rule inet filter input tcp dport 22 accept\n" +
		"delete rule inet filter input handle 11"
	if got != want {
		t.Errorf("\n  got:  %q\n  want: %q", got, want)
	}

	// Pop is LIFO (undo).
	popped, ok := cl.Pop()
	if !ok {
		t.Fatal("Pop on non-empty list returned !ok")
	}
	if _, isDelete := popped.(*DeleteRule); !isDelete {
		t.Errorf("popped op = %T, want *DeleteRule", popped)
	}
	if cl.Len() != 1 {
		t.Errorf("Len() after Pop = %d, want 1", cl.Len())
	}

	cl.Clear()
	if cl.Len() != 0 {
		t.Errorf("Len() after Clear = %d, want 0", cl.Len())
	}
	if _, ok := cl.Pop(); ok {
		t.Errorf("Pop on empty list returned ok=true")
	}
}

func TestOpsCopyIsDefensive(t *testing.T) {
	var cl ChangeList
	cl.Append(&AddRule{Family: model.FamilyINet, Table: "filter", Chain: "input", Body: "accept"})

	ops := cl.Ops()
	// Mutating the returned slice must not affect the ChangeList.
	ops[0] = &DeleteRule{Handle: 99}
	again := cl.Ops()
	if _, isAdd := again[0].(*AddRule); !isAdd {
		t.Errorf("ChangeList.Ops() returned a view, not a copy: %T", again[0])
	}
}

func TestDescribePrefix(t *testing.T) {
	// Sanity check that the staged-changes tree gets the right glyph.
	cases := []struct {
		op     Op
		prefix string
	}{
		{&AddRule{Family: model.FamilyINet, Table: "f", Chain: "i", Body: "accept"}, "+"},
		{&InsertRule{Family: model.FamilyINet, Table: "f", Chain: "i", Body: "accept"}, "+"},
		{&DeleteRule{Family: model.FamilyINet, Table: "f", Chain: "i", Handle: 1}, "-"},
		{&ReplaceRule{Family: model.FamilyINet, Table: "f", Chain: "i", Handle: 1, Body: "accept"}, "~"},
	}
	for _, c := range cases {
		if !strings.HasPrefix(c.op.Describe(), c.prefix) {
			t.Errorf("%T.Describe() = %q, want prefix %q", c.op, c.op.Describe(), c.prefix)
		}
	}
}
