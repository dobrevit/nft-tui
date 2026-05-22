package nft

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dobrevit/nft-tui/internal/model"
	"github.com/dobrevit/nft-tui/internal/staged"
)

// TestWriteStagedFile verifies the serialisation independently of the
// shell-out path; the file content drives both DryRun and Commit.
func TestWriteStagedFile(t *testing.T) {
	ops := []staged.Op{
		&staged.AddRule{
			Family: model.FamilyINet, Table: "filter", Chain: "input",
			Body: "tcp dport 22 accept", Comment: "ssh",
		},
		&staged.DeleteRule{
			Family: model.FamilyINet, Table: "filter", Chain: "input",
			Handle: 11,
		},
	}
	path, cleanup, err := writeStagedFile(ops)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	want := "" +
		`add rule inet filter input tcp dport 22 accept comment "ssh"` + "\n" +
		`delete rule inet filter input handle 11` + "\n"
	if got != want {
		t.Errorf("\n  got:\n%s\n  want:\n%s", got, want)
	}
}

// TestCommitNoChanges ensures an empty list short-circuits with
// ErrNoChanges before any shell-out is attempted (so the test passes
// even on a CI box without nft installed).
func TestCommitNoChanges(t *testing.T) {
	c := &Committer{NFTPath: "/nonexistent/should-not-be-invoked"}
	_, err := c.Commit(context.Background(), nil)
	if !errors.Is(err, ErrNoChanges) {
		t.Errorf("Commit(nil) = %v, want ErrNoChanges", err)
	}
}

// TestDryRunNoChanges similarly short-circuits with nil.
func TestDryRunNoChanges(t *testing.T) {
	c := &Committer{NFTPath: "/nonexistent/should-not-be-invoked"}
	if err := c.DryRun(context.Background(), nil); err != nil {
		t.Errorf("DryRun(nil) = %v, want nil", err)
	}
}

// TestArchive verifies the audit-file copy produces the expected
// timestamp-named file with the same content as the source.
func TestArchive(t *testing.T) {
	src := filepath.Join(t.TempDir(), "staged.nft")
	if err := os.WriteFile(src, []byte("add rule inet filter input accept\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "audit")
	dst, err := archive(src, dir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(filepath.Base(dst), "audit-") {
		t.Errorf("audit filename = %q, want prefix audit-", filepath.Base(dst))
	}
	if !strings.HasSuffix(dst, ".nft") {
		t.Errorf("audit filename = %q, want .nft suffix", dst)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	want := "add rule inet filter input accept\n"
	if string(got) != want {
		t.Errorf("archived content = %q, want %q", got, want)
	}
}

func TestDefaultAuditDir(t *testing.T) {
	// XDG_STATE_HOME takes precedence.
	t.Setenv("XDG_STATE_HOME", "/var/lib/nft-tui-state")
	if got, want := DefaultAuditDir(), "/var/lib/nft-tui-state/nft-tui"; got != want {
		t.Errorf("with XDG_STATE_HOME: got %q, want %q", got, want)
	}

	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("HOME", "/home/operator")
	if got, want := DefaultAuditDir(), "/home/operator/.local/state/nft-tui"; got != want {
		t.Errorf("with HOME only: got %q, want %q", got, want)
	}
}
