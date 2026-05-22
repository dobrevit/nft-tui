package nft

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/dobrevit/nft-tui/internal/staged"
)

// Committer is the shell-out path for applying staged changes to the
// kernel ruleset. It is stateless; methods are safe to call from any
// goroutine.
//
// We deliberately do NOT use the netlink connection (nft.Conn) for
// writes:
//   - DryRun via `nft -c -f` gives us nft's full validator for free.
//   - The serialised file IS the audit log; admins can paste it into
//     Ansible / a git repo.
//   - Raw-mode editing (F8 — type nft directly) needs the validator;
//     re-implementing nft's parser in Go is months of effort.
type Committer struct {
	// Path to the nft binary. Empty means look in $PATH.
	NFTPath string
	// AuditDir is where successful commits are archived as
	// audit-YYYYMMDD-HHMMSS.nft. Empty disables archiving (the temp
	// file is removed after commit).
	AuditDir string
}

// DryRun validates ops by running `nft -c -f <tempfile>`. It returns a
// nil error iff the staged changes would apply cleanly against the
// current kernel state. The error wraps nft's stderr so the UI can show
// the parser's line/column information verbatim.
//
// An empty ChangeList is a no-op success.
func (c *Committer) DryRun(ctx context.Context, ops []staged.Op) error {
	if len(ops) == 0 {
		return nil
	}
	path, cleanup, err := writeStagedFile(ops)
	if err != nil {
		return err
	}
	defer cleanup()
	return c.run(ctx, "-c", "-f", path)
}

// Commit applies the staged changes atomically. On success it returns
// the path of the archived audit file (or "" if AuditDir is empty); on
// failure it returns an error wrapping nft's output. A successful
// commit is a single nftables transaction — partial application is
// impossible by virtue of nft -f's semantics.
//
// Commit refuses to apply an empty ChangeList (returns ErrNoChanges) so
// the caller doesn't accidentally archive an empty audit entry.
func (c *Committer) Commit(ctx context.Context, ops []staged.Op) (auditPath string, err error) {
	if len(ops) == 0 {
		return "", ErrNoChanges
	}
	path, cleanup, err := writeStagedFile(ops)
	if err != nil {
		return "", err
	}
	defer cleanup()

	// Validate first. If it fails we don't apply and don't archive.
	if err := c.run(ctx, "-c", "-f", path); err != nil {
		return "", fmt.Errorf("dry-run failed before commit: %w", err)
	}
	if err := c.run(ctx, "-f", path); err != nil {
		return "", fmt.Errorf("commit failed: %w", err)
	}
	if c.AuditDir == "" {
		return "", nil
	}
	return archive(path, c.AuditDir)
}

// ErrNoChanges signals that Commit was called with no staged ops.
var ErrNoChanges = errors.New("no staged changes to commit")

// run invokes nft with the supplied args, capturing combined output and
// returning it wrapped in an error on non-zero exit. Context cancellation
// kills the child.
func (c *Committer) run(ctx context.Context, args ...string) error {
	bin := c.NFTPath
	if bin == "" {
		bin = "nft"
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	// nft's error messages are already operator-friendly; surface them
	// verbatim alongside the exit error for the UI to display.
	return fmt.Errorf("%s %s: %w\n%s", bin, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
}

// writeStagedFile serialises ops to a temp file and returns its path
// plus a cleanup function that removes the file. The caller MUST call
// cleanup; archive() copies the file into the audit dir before that.
func writeStagedFile(ops []staged.Op) (string, func(), error) {
	f, err := os.CreateTemp("", "nft-tui-staged-*.nft")
	if err != nil {
		return "", nil, fmt.Errorf("create staged file: %w", err)
	}
	for i, op := range ops {
		if i > 0 {
			if _, err := f.WriteString("\n"); err != nil {
				f.Close()
				os.Remove(f.Name())
				return "", nil, err
			}
		}
		if _, err := f.WriteString(op.NFT()); err != nil {
			f.Close()
			os.Remove(f.Name())
			return "", nil, err
		}
	}
	if _, err := f.WriteString("\n"); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", nil, err
	}
	if err := f.Close(); err != nil {
		os.Remove(f.Name())
		return "", nil, err
	}
	path := f.Name()
	cleanup := func() { _ = os.Remove(path) }
	return path, cleanup, nil
}

// archive copies src into auditDir as audit-YYYYMMDD-HHMMSS.nft.
// The destination directory is created if missing.
func archive(src, auditDir string) (string, error) {
	if err := os.MkdirAll(auditDir, 0o750); err != nil {
		return "", fmt.Errorf("create audit dir: %w", err)
	}
	dst := filepath.Join(auditDir,
		fmt.Sprintf("audit-%s.nft", time.Now().Format("20060102-150405")))
	if err := copyFile(src, dst); err != nil {
		return "", fmt.Errorf("archive audit file: %w", err)
	}
	return dst, nil
}

// copyFile is the no-frills file-copy helper used by archive.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o640)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(dst)
		return err
	}
	return out.Close()
}

// DefaultAuditDir returns the recommended audit directory:
// $XDG_STATE_HOME/nft-tui or $HOME/.local/state/nft-tui.
func DefaultAuditDir() string {
	if d := os.Getenv("XDG_STATE_HOME"); d != "" {
		return filepath.Join(d, "nft-tui")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "nft-tui")
	}
	return filepath.Join(home, ".local", "state", "nft-tui")
}
