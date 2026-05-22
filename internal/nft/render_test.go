package nft

import (
	"encoding/binary"
	"testing"

	"github.com/google/nftables"
	"github.com/google/nftables/expr"

	"github.com/dobrevit/nft-tui/internal/model"
)

// be16 returns the big-endian byte representation of u as a 2-byte slice.
func be16(u uint16) []byte {
	b := make([]byte, 2)
	binary.BigEndian.PutUint16(b, u)
	return b
}

// le32 returns the little-endian byte representation of u as a 4-byte slice.
// Used for ct-state masks and similar 32-bit values.
func le32(u uint32) []byte {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, u)
	return b
}

// padifname returns a NUL-padded IFNAMSIZ (16-byte) buffer holding name —
// the wire form the kernel uses for iifname/oifname comparisons.
func padifname(name string) []byte {
	b := make([]byte, 16)
	copy(b, name)
	return b
}

func TestRenderRule(t *testing.T) {
	cases := []struct {
		name  string
		exprs []expr.Any
		sets  map[string]*model.Set
		want  string
	}{
		{
			name: "iifname accept",
			exprs: []expr.Any{
				&expr.Meta{Key: expr.MetaKeyIIFNAME, Register: 1},
				&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: padifname("lo")},
				&expr.Verdict{Kind: expr.VerdictAccept},
			},
			want: `iifname "lo" accept`,
		},
		{
			name: "ct state established,related accept",
			exprs: []expr.Any{
				&expr.Ct{Key: expr.CtKeySTATE, Register: 1},
				// mask covers ESTABLISHED (0x02) | RELATED (0x04) = 0x06
				&expr.Bitwise{
					SourceRegister: 1, DestRegister: 1, Len: 4,
					Mask: le32(0x06),
					Xor:  le32(0),
				},
				&expr.Cmp{Op: expr.CmpOpNeq, Register: 1, Data: le32(0)},
				&expr.Verdict{Kind: expr.VerdictAccept},
			},
			want: `ct state { established, related } accept`,
		},
		{
			name: "ip saddr /24 prefix accept (truncated payload encoding)",
			exprs: []expr.Any{
				// nft optimises /24 into a 3-byte payload load + 3-byte cmp.
				&expr.Payload{
					DestRegister: 1, Base: expr.PayloadBaseNetworkHeader,
					Offset: 12, Len: 3,
				},
				&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: []byte{10, 0, 0}},
				&expr.Verdict{Kind: expr.VerdictAccept},
			},
			want: `ip saddr 10.0.0.0/24 accept`,
		},
		{
			name: "nfproto ipv4 guard elided before ip saddr",
			exprs: []expr.Any{
				&expr.Meta{Key: expr.MetaKeyNFPROTO, Register: 1},
				&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: []byte{2}},
				&expr.Payload{
					DestRegister: 1, Base: expr.PayloadBaseNetworkHeader,
					Offset: 12, Len: 4,
				},
				&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: []byte{10, 0, 0, 1}},
				&expr.Verdict{Kind: expr.VerdictAccept},
			},
			want: `ip saddr 10.0.0.1 accept`,
		},
		{
			name: "nfproto guard kept when next match is not ip-family",
			exprs: []expr.Any{
				&expr.Meta{Key: expr.MetaKeyNFPROTO, Register: 1},
				&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: []byte{2}},
				&expr.Counter{Packets: 0, Bytes: 0},
				&expr.Verdict{Kind: expr.VerdictAccept},
			},
			want: `meta nfproto ipv4 counter packets 0 bytes 0 accept`,
		},
		{
			name: "tcp dport equals 22 accept",
			exprs: []expr.Any{
				&expr.Meta{Key: expr.MetaKeyL4PROTO, Register: 1},
				&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: []byte{6}},
				&expr.Payload{
					DestRegister: 1, Base: expr.PayloadBaseTransportHeader,
					Offset: 2, Len: 2,
				},
				&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: be16(22)},
				&expr.Verdict{Kind: expr.VerdictAccept},
			},
			want: `tcp dport 22 accept`,
		},
		{
			name: "tcp dport anonymous set inlined",
			exprs: []expr.Any{
				&expr.Meta{Key: expr.MetaKeyL4PROTO, Register: 1},
				&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: []byte{6}},
				&expr.Payload{
					DestRegister: 1, Base: expr.PayloadBaseTransportHeader,
					Offset: 2, Len: 2,
				},
				&expr.Lookup{SourceRegister: 1, SetName: "__set0"},
				&expr.Counter{Packets: 4, Bytes: 240},
				&expr.Verdict{Kind: expr.VerdictAccept},
			},
			sets: map[string]*model.Set{
				"__set0": {
					Name:    "__set0",
					KeyType: "inet_service",
					Elements: []model.SetElement{
						{Key: "22"}, {Key: "80"}, {Key: "443"},
					},
				},
			},
			want: `tcp dport { 22, 80, 443 } counter packets 4 bytes 240 accept`,
		},
		{
			name: "named set lookup not inlined",
			exprs: []expr.Any{
				&expr.Payload{
					DestRegister: 1, Base: expr.PayloadBaseNetworkHeader,
					Offset: 12, Len: 4,
				},
				&expr.Lookup{SourceRegister: 1, SetName: "blacklist_v4"},
				&expr.Verdict{Kind: expr.VerdictDrop},
			},
			want: `ip saddr @blacklist_v4 drop`,
		},
		{
			name: "log prefix + counter, no verdict",
			exprs: []expr.Any{
				&expr.Log{Data: []byte("dropped: ")},
				&expr.Counter{Packets: 17, Bytes: 1234},
			},
			want: `log prefix "dropped: " counter packets 17 bytes 1234`,
		},
		{
			name: "masquerade",
			exprs: []expr.Any{
				&expr.Meta{Key: expr.MetaKeyOIFNAME, Register: 1},
				&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: padifname("eth0")},
				&expr.Masq{},
			},
			want: `oifname "eth0" masquerade`,
		},
		{
			name: "reject with tcp reset",
			exprs: []expr.Any{
				&expr.Reject{Type: 1, Code: 0},
			},
			want: `reject with tcp reset`,
		},
		{
			name: "jump to chain",
			exprs: []expr.Any{
				&expr.Verdict{Kind: expr.VerdictJump, Chain: "LOG_AND_DROP"},
			},
			want: `jump LOG_AND_DROP`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := &nftables.Rule{Exprs: tc.exprs}
			got := RenderRule(r, tc.sets).NFT
			if got != tc.want {
				t.Errorf("\n  got:  %q\n  want: %q", got, tc.want)
			}
		})
	}
}

// TestRenderRule_DecodesColumns checks that the decoded fields populated
// for the columnar list view come out correctly.
func TestRenderRule_DecodesColumns(t *testing.T) {
	r := &nftables.Rule{
		Exprs: []expr.Any{
			&expr.Meta{Key: expr.MetaKeyIIFNAME, Register: 1},
			&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: padifname("eth0")},
			&expr.Meta{Key: expr.MetaKeyOIFNAME, Register: 1},
			&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: padifname("eth1")},
			&expr.Payload{
				DestRegister: 1, Base: expr.PayloadBaseNetworkHeader,
				Offset: 12, Len: 4,
			},
			&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: []byte{10, 0, 0, 1}},
			&expr.Meta{Key: expr.MetaKeyL4PROTO, Register: 1},
			&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: []byte{6}},
			&expr.Payload{
				DestRegister: 1, Base: expr.PayloadBaseTransportHeader,
				Offset: 2, Len: 2,
			},
			&expr.Cmp{Op: expr.CmpOpEq, Register: 1, Data: be16(443)},
			&expr.Counter{Packets: 100, Bytes: 50000},
			&expr.Verdict{Kind: expr.VerdictAccept},
		},
	}
	got := RenderRule(r, nil)

	checks := map[string]string{
		"IIfName": got.IIfName,
		"OIfName": got.OIfName,
		"SAddr":   got.SAddr,
		"Proto":   got.Proto,
		"DPort":   got.DPort,
		"Verdict": got.Verdict,
	}
	want := map[string]string{
		"IIfName": "eth0",
		"OIfName": "eth1",
		"SAddr":   "10.0.0.1",
		"Proto":   "tcp",
		"DPort":   "443",
		"Verdict": "accept",
	}
	for k, w := range want {
		if checks[k] != w {
			t.Errorf("%s: got %q, want %q", k, checks[k], w)
		}
	}
	if !got.HasCounter || got.CounterPackets != 100 || got.CounterBytes != 50000 {
		t.Errorf("counter not captured: %+v", got)
	}
}
