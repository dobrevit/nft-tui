package nft

import (
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/google/nftables"
	"github.com/google/nftables/userdata"

	"github.com/dobrevit/nft-tui/internal/model"
)

// Conn wraps a netlink connection to the kernel's nftables subsystem.
// The zero value is not usable; call NewConn.
//
// Methods on Conn are serialised by an internal mutex; the underlying
// netlink socket is not designed for concurrent use.
type Conn struct {
	mu sync.Mutex
	c  *nftables.Conn
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
// renders rules into our model. Set elements are fetched eagerly so the
// renderer can inline anonymous sets (`__set%d`) into the rule text.
func (c *Conn) ListRuleset() (*model.Ruleset, error) {
	if c == nil {
		return nil, errors.New("nft.Conn is nil")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.c == nil {
		return nil, errors.New("nft.Conn is closed")
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
			mt, err := c.readTable(t, fam)
			if err != nil {
				return nil, err
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

// readTable fetches one table's sets (with elements) and chains (with rules).
// Sets are fetched first so the rule renderer can resolve anonymous-set
// lookups against their element lists.
func (c *Conn) readTable(t *nftables.Table, fam nftables.TableFamily) (*model.Table, error) {
	mt := &model.Table{
		Family: model.Family(familyName(fam)),
		Name:   t.Name,
	}

	sets, err := c.c.GetSets(t)
	if err != nil {
		return nil, fmt.Errorf("list sets (%s/%s): %w", mt.Family, t.Name, err)
	}
	setIndex := make(map[string]*model.Set, len(sets))
	for _, s := range sets {
		ms, err := c.readSet(s, mt)
		if err != nil {
			return nil, err
		}
		mt.Sets = append(mt.Sets, ms)
		setIndex[s.Name] = ms
	}

	chains, err := c.c.ListChainsOfTableFamily(fam)
	if err != nil {
		return nil, fmt.Errorf("list chains (%s): %w", mt.Family, err)
	}
	for _, ch := range chains {
		if ch.Table.Name != t.Name {
			continue
		}
		mc := convertChain(ch, mt)
		rules, err := c.c.GetRules(t, ch)
		if err != nil {
			return nil, fmt.Errorf("list rules (%s/%s/%s): %w",
				mt.Family, t.Name, ch.Name, err)
		}
		for _, r := range rules {
			mc.Rules = append(mc.Rules, convertRule(r, mc, setIndex))
		}
		mt.Chains = append(mt.Chains, mc)
	}

	return mt, nil
}

// readSet fetches a set's metadata and elements.
func (c *Conn) readSet(s *nftables.Set, t *model.Table) (*model.Set, error) {
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
	els, err := c.c.GetSetElements(s)
	if err != nil {
		// A set may legitimately have no elements; only surface real errors.
		return nil, fmt.Errorf("list set elements (%s/%s/%s): %w",
			t.Family, t.Name, s.Name, err)
	}
	ms.Elements = make([]model.SetElement, 0, len(els))
	for _, e := range els {
		ms.Elements = append(ms.Elements, convertSetElement(s.KeyType.Name, e))
	}
	ms.Size = len(ms.Elements)
	return ms, nil
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

func convertRule(r *nftables.Rule, ch *model.Chain, sets map[string]*model.Set) *model.Rule {
	rendered := RenderRule(r, sets)
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
	if c, ok := userdata.GetString(r.UserData, userdata.TypeComment); ok && c != "" {
		mr.Comment = c
		// nft's textual form puts the comment at the end of the rule.
		mr.NFT = mr.NFT + ` comment "` + c + `"`
	}
	return mr
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
