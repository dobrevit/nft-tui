package nft

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAuditEntryFormat_Commit(t *testing.T) {
	e := AuditEntry{
		Action: "commit",
		UID:    1000,
		User:   "alice",
		Ops:    2,
		File:   "audit-20260522-134203.123.nft",
		Body:   "add rule inet filter input tcp dport 22 accept\ndelete rule inet filter input handle 11\n",
		At:     time.Date(2026, 5, 22, 13, 42, 3, 0, time.UTC),
	}
	got := e.format()
	wantHeader := "# 2026-05-22T13:42:03Z  uid=1000 (alice)  action=commit  ops=2  file=audit-20260522-134203.123.nft\n"
	if !strings.HasPrefix(got, wantHeader) {
		t.Errorf("header wrong:\n  got:  %q\n  want prefix: %q", got, wantHeader)
	}
	if !strings.Contains(got, "tcp dport 22 accept") {
		t.Errorf("missing body: %q", got)
	}
	if !strings.HasSuffix(got, "\n\n") {
		t.Errorf("missing trailing blank-line separator: %q", got)
	}
}

func TestAuditEntryFormat_Restore(t *testing.T) {
	e := AuditEntry{
		Action: "restore",
		UID:    1000,
		User:   "alice",
		File:   "/home/alice/snap.nft",
		At:     time.Date(2026, 5, 22, 13, 42, 3, 0, time.UTC),
	}
	got := e.format()
	// No Ops, no Body — just the header + blank line.
	want := "# 2026-05-22T13:42:03Z  uid=1000 (alice)  action=restore  file=/home/alice/snap.nft\n\n"
	if got != want {
		t.Errorf("\n  got:  %q\n  want: %q", got, want)
	}
}

func TestAuditEntryFormat_OmitsEmptyFields(t *testing.T) {
	// User missing (e.g. container without /etc/passwd) — shouldn't emit
	// the empty parens.
	e := AuditEntry{
		Action: "rollback",
		UID:    0,
		File:   "/tmp/rb.nft",
		At:     time.Date(2026, 5, 22, 13, 42, 3, 0, time.UTC),
	}
	got := e.format()
	if strings.Contains(got, "()") {
		t.Errorf("empty user produced empty parens: %q", got)
	}
}

func TestAuditAppendsToLog(t *testing.T) {
	dir := t.TempDir()
	c := &Committer{AuditDir: dir}
	if err := c.Audit(AuditEntry{Action: "commit", UID: 1000, User: "alice", Ops: 1, Body: "accept\n"}); err != nil {
		t.Fatal(err)
	}
	if err := c.Audit(AuditEntry{Action: "restore", UID: 1000, User: "alice", File: "/x/snap.nft"}); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(dir, auditFilename))
	if err != nil {
		t.Fatal(err)
	}
	// Two `#`-prefixed header lines, in order.
	headers := 0
	for _, line := range strings.Split(string(body), "\n") {
		if strings.HasPrefix(line, "# ") {
			headers++
		}
	}
	if headers != 2 {
		t.Errorf("expected 2 header lines, got %d\n--- log ---\n%s", headers, body)
	}
	if !strings.Contains(string(body), "action=commit") {
		t.Errorf("missing commit entry:\n%s", body)
	}
	if !strings.Contains(string(body), "action=restore") {
		t.Errorf("missing restore entry:\n%s", body)
	}
}

func TestAuditWithoutDirIsNoOp(t *testing.T) {
	c := &Committer{} // no AuditDir
	if err := c.Audit(AuditEntry{Action: "commit"}); err != nil {
		t.Errorf("Audit with empty AuditDir should be a no-op, got %v", err)
	}
}
