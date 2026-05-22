package nft

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/google/nftables"

	"github.com/dobrevit/nft-tui/internal/model"
)

// Conn wraps a netlink connection to the kernel's nftables subsystem.
// The zero value is not usable; call NewConn.
type Conn struct {
	c *nftables.Conn
}

// NewConn opens a netlink connection. On a host without CAP_NET_ADMIN this
// returns an error. For development, run inside an unshared netns:
//
//	unshare -rn ./nft-tui
func NewConn() (*Conn, error) {
	c, err := nftables.New()
	if err != nil {
		return nil, fmt.Errorf("open nftables netlink connection: %w", err)
	}
	return &Conn{c: c}, nil
}

// Close releases the netlink connection.
func (c *Conn) Close() error {
	if c == nil || c.c == nil {
		return nil
	}
	return c.c.CloseLasting()
}

// ListRuleset fetches the entire ruleset (all families, all tables) and
// renders rules into our model. One netlink round-trip per logical query
// (tables, chains-per-table, rules-per-chain, sets-per-table).
func (c *Conn) ListRuleset() (*model.Ruleset, error) {
	if c == nil || c.c == nil {
		return nil, errors.New("nft.Conn is nil")
	}

	families := []nftables.TableFamily{
		nftables.TableFamilyIPv4,
		nftables.TableFamilyIPv6,
		nftables.TableFamilyINet,
		nftables.TableFamilyARP,
		nftables.TableFamilyBridge,
		nftables.TableFamilyNetdev,
	}

	rs := &model.Ruleset{FetchedAt: time.Now()}

	for _, fam := range families {
		tables, err := c.c.ListTablesOfFamily(fam)
		if err != nil {
			return nil, fmt.Errorf("list tables (family=%s): %w", familyName(fam), err)
		}
		for _, t := range tables {
			mt := &model.Table{
				Family: model.Family(familyName(fam)),
				Name:   t.Name,
			}

			chains, err := c.c.ListChainsOfTableFamily(fam)
			if err != nil {
				return nil, fmt.Errorf("list chains (family=%s): %w", familyName(fam), err)
			}
			for _, ch := range chains {
				if ch.Table.Name != t.Name {
					continue
				}
				mc := convertChain(ch, mt)
				rules, err := c.c.GetRules(t, ch)
				if err != nil {
					return nil, fmt.Errorf("list rules (%s/%s/%s): %w",
						familyName(fam), t.Name, ch.Name, err)
				}
				for _, r := range rules {
					mc.Rules = append(mc.Rules, convertRule(r, mc))
				}
				mt.Chains = append(mt.Chains, mc)
			}

			sets, err := c.c.GetSets(t)
			if err != nil {
				return nil, fmt.Errorf("list sets (%s/%s): %w",
					familyName(fam), t.Name, err)
			}
			for _, s := range sets {
				mt.Sets = append(mt.Sets, convertSet(s, mt))
			}

			rs.Tables = append(rs.Tables, mt)
		}
	}

	sort.SliceStable(rs.Tables, func(i, j int) bool {
		if rs.Tables[i].Family != rs.Tables[j].Family {
			return rs.Tables[i].Family < rs.Tables[j].Family
		}
		return rs.Tables[i].Name < rs.Tables[j].Name
	})

	return rs, nil
}

func convertChain(c *nftables.Chain, t *model.Table) *model.Chain {
	mc := &model.Chain{
		Table: t,
		Name:  c.Name,
	}
	if c.Hooknum != nil {
		mc.IsBase = true
		mc.Hook = hookName(*c.Hooknum)
		mc.Type = string(c.Type)
		if c.Priority != nil {
			mc.Priority = int32(*c.Priority)
		}
		if c.Policy != nil {
			switch *c.Policy {
			case nftables.ChainPolicyAccept:
				mc.Policy = "accept"
			case nftables.ChainPolicyDrop:
				mc.Policy = "drop"
			}
		}
		mc.Device = c.Device
	}
	return mc
}

func convertRule(r *nftables.Rule, ch *model.Chain) *model.Rule {
	rendered := RenderRule(r)
	mr := &model.Rule{
		Chain:   ch,
		Handle:  r.Handle,
		NFT:     rendered.NFT,
		IIfName: rendered.IIfName,
		OIfName: rendered.OIfName,
		SAddr:   rendered.SAddr,
		DAddr:   rendered.DAddr,
		SPort:   rendered.SPort,
		DPort:   rendered.DPort,
		Proto:   rendered.Proto,
		CTState: rendered.CTState,
		Verdict: rendered.Verdict,
	}
	if rendered.HasCounter {
		mr.Counter = model.Counter{
			Packets: rendered.CounterPackets,
			Bytes:   rendered.CounterBytes,
			Present: true,
		}
	}
	if len(r.UserData) > 0 {
		// google/nftables exposes UserData as a TLV; the comment is encoded
		// there. Decoding is best-effort — empty string if absent.
		mr.Comment = decodeComment(r.UserData)
	}
	return mr
}

func convertSet(s *nftables.Set, t *model.Table) *model.Set {
	ms := &model.Set{
		Table:   t,
		Name:    s.Name,
		KeyType: s.KeyType.Name,
		Flags: model.SetFlags{
			Constant: s.Constant,
			Dynamic:  s.Dynamic,
			Interval: s.Interval,
			Counter:  s.Counter,
			Timeout:  s.HasTimeout,
		},
		Timeout: s.Timeout,
	}
	return ms
}

// hookName converts a kernel hook number to its nft name.
func hookName(h nftables.ChainHook) string {
	switch h {
	case *nftables.ChainHookPrerouting:
		return "prerouting"
	case *nftables.ChainHookInput:
		return "input"
	case *nftables.ChainHookForward:
		return "forward"
	case *nftables.ChainHookOutput:
		return "output"
	case *nftables.ChainHookPostrouting:
		return "postrouting"
	case *nftables.ChainHookIngress:
		return "ingress"
	case *nftables.ChainHookEgress:
		return "egress"
	}
	return fmt.Sprintf("hook<%d>", h)
}

func familyName(f nftables.TableFamily) string {
	switch f {
	case nftables.TableFamilyIPv4:
		return "ip"
	case nftables.TableFamilyIPv6:
		return "ip6"
	case nftables.TableFamilyINet:
		return "inet"
	case nftables.TableFamilyARP:
		return "arp"
	case nftables.TableFamilyBridge:
		return "bridge"
	case nftables.TableFamilyNetdev:
		return "netdev"
	}
	return fmt.Sprintf("family<%d>", f)
}

// decodeComment walks a TLV-encoded user-data blob and returns the comment
// attribute (NFTNL_UDATA_RULE_COMMENT = 0). Returns "" if not present or if
// the encoding looks malformed.
func decodeComment(ud []byte) string {
	for i := 0; i+2 <= len(ud); {
		typ := ud[i]
		length := int(ud[i+1])
		if i+2+length > len(ud) {
			return ""
		}
		val := ud[i+2 : i+2+length]
		if typ == 0 { // NFTNL_UDATA_RULE_COMMENT
			s := string(val)
			// kernel zero-terminates the string
			for j := 0; j < len(s); j++ {
				if s[j] == 0 {
					return s[:j]
				}
			}
			return s
		}
		i += 2 + length
	}
	return ""
}
