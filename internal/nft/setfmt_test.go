package nft

import (
	"strings"
	"testing"

	"github.com/dobrevit/nft-tui/internal/model"
)

func TestRenderMapElements_PlainMap(t *testing.T) {
	s := &model.Set{
		IsMap:    true,
		KeyType:  "ifname",
		DataType: "verdict",
		Elements: []model.SetElement{
			{Key: `"eth0"`, Value: "jump LAN"},
			{Key: `"eth1"`, Value: "jump WAN"},
		},
	}
	got := strings.Join(renderMapElements(s), " | ")
	want := `"eth0" : jump LAN | "eth1" : jump WAN`
	if got != want {
		t.Errorf("\n  got:  %q\n  want: %q", got, want)
	}
}

func TestRenderMapElements_IntervalKeyed(t *testing.T) {
	// `{ 1024-2048 : jump LAN, 5000-6000 : jump DMZ }` arrives as
	// four elements: start, end-sentinel, start, end-sentinel.
	s := &model.Set{
		IsMap:    true,
		KeyType:  "inet_service",
		DataType: "verdict",
		Elements: []model.SetElement{
			{Key: "1024", Value: "jump LAN"},
			{Key: "2048", IntervalEnd: true},
			{Key: "5000", Value: "jump DMZ"},
			{Key: "6000", IntervalEnd: true},
		},
	}
	got := strings.Join(renderMapElements(s), " | ")
	want := "1024-2048 : jump LAN | 5000-6000 : jump DMZ"
	if got != want {
		t.Errorf("\n  got:  %q\n  want: %q", got, want)
	}
}

func TestRenderMapElements_MissingValueSurfacedNotDropped(t *testing.T) {
	s := &model.Set{
		IsMap: true, KeyType: "inet_service", DataType: "mark",
		Elements: []model.SetElement{
			{Key: "22"}, // no Value populated
		},
	}
	got := strings.Join(renderMapElements(s), " | ")
	if !strings.Contains(got, "<missing-value>") {
		t.Errorf("missing-value sentinel not surfaced; got %q", got)
	}
}

func TestRenderMapElements_DefensiveAgainstOrphans(t *testing.T) {
	// Pathological: an IntervalEnd with no preceding start, and a start
	// with no following end. Neither should panic or drop information.
	s := &model.Set{
		IsMap: true, KeyType: "inet_service", DataType: "verdict",
		Elements: []model.SetElement{
			{Key: "9999", IntervalEnd: true}, // orphan end (ignored)
			{Key: "22", Value: "accept"},     // bare start, no end
		},
	}
	got := strings.Join(renderMapElements(s), " | ")
	if !strings.Contains(got, "22 : accept") {
		t.Errorf("dangling start not flushed; got %q", got)
	}
}
