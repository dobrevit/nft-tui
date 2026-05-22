package ui

import (
	"testing"

	"github.com/dobrevit/nft-tui/internal/model"
)

func TestFormFieldsFromRule(t *testing.T) {
	r := &model.Rule{
		IIfName: "eth0",
		OIfName: "eth1",
		SAddr:   "10.0.0.0/24",
		DAddr:   "192.0.2.7",
		Proto:   "tcp",
		SPort:   "1024",
		DPort:   "22",
		CTState: "{ established, related }",
		Verdict: "accept",
		Counter: model.Counter{Present: true},
	}
	ff := formFieldsFromRule(r)

	if ff.iifname != "eth0" {
		t.Errorf("iifname: got %q", ff.iifname)
	}
	if ff.oifname != "eth1" {
		t.Errorf("oifname: got %q", ff.oifname)
	}
	if ff.saddr != "10.0.0.0/24" {
		t.Errorf("saddr: got %q", ff.saddr)
	}
	if ff.daddr != "192.0.2.7" {
		t.Errorf("daddr: got %q", ff.daddr)
	}
	if ff.proto != "tcp" {
		t.Errorf("proto: got %q", ff.proto)
	}
	if ff.sport != "1024" || ff.dport != "22" {
		t.Errorf("ports: got sport=%q dport=%q", ff.sport, ff.dport)
	}
	if ff.verdict != "accept" {
		t.Errorf("verdict: got %q", ff.verdict)
	}
	if !ff.counter {
		t.Errorf("counter: got false, want true")
	}
	// ct established + related set, new + invalid clear
	if !ff.ctStates[0] {
		t.Errorf("ctState[established] not set")
	}
	if !ff.ctStates[1] {
		t.Errorf("ctState[related] not set")
	}
	if ff.ctStates[2] || ff.ctStates[3] {
		t.Errorf("ctState[new/invalid] should be clear: %v", ff.ctStates)
	}
}

func TestFormFieldsFromRuleSingleCTState(t *testing.T) {
	// Renderer emits a bare name (no braces) when only one state is
	// selected — `ct state established` rather than `ct state { … }`.
	r := &model.Rule{CTState: "established"}
	ff := formFieldsFromRule(r)
	if !ff.ctStates[0] {
		t.Errorf("single-state established not set: %v", ff.ctStates)
	}
}

func TestFormFieldsFromRuleEmptyProto(t *testing.T) {
	// Renderer leaves Proto empty when no L4-proto context was set.
	// formFieldsFromRule maps that onto the dropdown's "any" option
	// so the form's regenerateBody doesn't include port clauses.
	r := &model.Rule{}
	ff := formFieldsFromRule(r)
	if ff.proto != "any" {
		t.Errorf("empty Proto should map to 'any', got %q", ff.proto)
	}
}

func TestFormFieldsRoundTrip(t *testing.T) {
	// A rule built by formFieldsFromRule then immediately rendered
	// back via regenerateBody should produce a body that matches the
	// canonical form. (Not byte-for-byte with the original NFT
	// because the renderer doesn't necessarily preserve token order
	// for set-element matches, etc., but the structural pieces
	// should appear.)
	r := &model.Rule{
		IIfName: "eth0",
		Proto:   "tcp",
		DPort:   "22",
		Verdict: "accept",
		Counter: model.Counter{Present: true},
	}
	ff := formFieldsFromRule(r)
	body := ff.regenerateBody()
	want := `iifname "eth0" tcp dport 22 counter accept`
	if body != want {
		t.Errorf("round-trip body mismatch:\n  got:  %q\n  want: %q", body, want)
	}
}
