package nft

import (
	"bytes"
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
	archived, err := archive(path, c.AuditDir)
	if err != nil {
		return "", err
	}
	// Also append a structured entry to the rolling audit.log. Best
	// effort: a failure here doesn't undo the (successful) commit;
	// surface the error so the UI can warn but don't roll back.
	uid, user := currentOperator()
	if auditErr := c.Audit(AuditEntry{
		Action: "commit", UID: uid, User: user,
		Ops:  len(ops),
		File: filepath.Base(archived),
		Body: scriptFromOps(ops),
	}); auditErr != nil {
		return archived, fmt.Errorf("commit succeeded but audit log write failed: %w", auditErr)
	}
	return archived, nil
}

// scriptFromOps renders the staged Op list as the canonical nft
// script. Mirrors what writeStagedFile produces. Used for the audit
// log body so the audit file contains the exact text applied (the
// archived file already has it, but inlining keeps `grep` workflows
// snappy: one file, every change, no joining).
func scriptFromOps(ops []staged.Op) string {
	var b strings.Builder
	for _, op := range ops {
		b.WriteString(op.NFT())
		b.WriteByte('\n')
	}
	return b.String()
}

// ErrNoChanges signals that Commit was called with no staged ops.
var ErrNoChanges = errors.New("no staged changes to commit")

// Snapshot writes the current kernel ruleset to path as a self-contained
// nft script. The output is exactly what `nft list ruleset` produces,
// preceded by a header comment block (timestamp, host, operator UID)
// so the file is self-documenting when grepped or git-committed.
//
// `nft list ruleset` is canonical and round-trips through `nft -f`, so
// a Snapshot followed by a Restore is identity on the structural
// content. (Counters are lost on restore — they're not part of the
// ruleset declaration.)
func (c *Committer) Snapshot(ctx context.Context, path string) error {
	bin := c.bin()
	out, err := exec.CommandContext(ctx, bin, "list", "ruleset").Output()
	if err != nil {
		return fmt.Errorf("%s list ruleset: %w", bin, exitDetail(err))
	}

	host, _ := os.Hostname()
	header := fmt.Sprintf(
		"# nft-tui snapshot\n# created: %s\n# host:    %s\n# uid:     %d\n\n",
		time.Now().Format(time.RFC3339), host, os.Getuid(),
	)
	body := append([]byte(header), out...)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		return fmt.Errorf("write snapshot %s: %w", path, err)
	}
	return nil
}

// Restore replaces the live ruleset with the contents of path,
// atomically. The kernel-side sequence is:
//
//	flush ruleset
//	<contents of path>
//
// applied as a single `nft -f` transaction so there is never a window
// with no rules. The snapshot is dry-run first via `nft -c`; the
// kernel is not touched if validation fails.
//
// CAUTION: this is the most destructive operation in the tool. The UI
// wraps it in a 60-second dead-man's switch (see internal/ui/deadman).
// Direct callers must understand they're committing to "wipe and
// replace" semantics.
func (c *Committer) Restore(ctx context.Context, path string) error {
	body, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read snapshot %s: %w", path, err)
	}
	script := append([]byte("flush ruleset\n"), body...)

	if err := c.runStdin(ctx, script, "-c", "-f", "-"); err != nil {
		return fmt.Errorf("restore dry-run failed: %w", err)
	}
	if err := c.runStdin(ctx, script, "-f", "-"); err != nil {
		return fmt.Errorf("restore failed: %w", err)
	}
	return nil
}

// bin returns the resolved nft binary path. Empty NFTPath means
// "look in $PATH".
func (c *Committer) bin() string {
	if c.NFTPath != "" {
		return c.NFTPath
	}
	return "nft"
}

// runStdin invokes nft with args and feeds `script` on stdin, capturing
// combined output and surfacing it on non-zero exit.
func (c *Committer) runStdin(ctx context.Context, script []byte, args ...string) error {
	cmd := exec.CommandContext(ctx, c.bin(), args...)
	cmd.Stdin = bytes.NewReader(script)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s %s: %w\n%s",
		c.bin(), strings.Join(args, " "), err,
		strings.TrimSpace(string(out)))
}

// exitDetail enriches an *exec.ExitError with its captured stderr
// (if any), so callers see what nft actually said.
func exitDetail(err error) error {
	var ee *exec.ExitError
	if errors.As(err, &ee) && len(ee.Stderr) > 0 {
		return fmt.Errorf("%w\n%s", err, strings.TrimSpace(string(ee.Stderr)))
	}
	return err
}

// run invokes nft with the supplied args, capturing combined output and
// returning it wrapped in an error on non-zero exit. Context cancellation
// kills the child.
func (c *Committer) run(ctx context.Context, args ...string) error {
	bin := c.bin()
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
func writeStagedFile(ops []staged.Op) (path string, cleanup func(), err error) {
	f, err := os.CreateTemp("", "nft-tui-staged-*.nft")
	if err != nil {
		return "", nil, fmt.Errorf("create staged file: %w", err)
	}
	// On any write failure: close + remove the temp file. Failures from
	// those cleanup calls are not useful to surface — the operator
	// can't act on them and the original write error is the real news.
	bail := func(werr error) (string, func(), error) {
		_ = f.Close()
		_ = os.Remove(f.Name())
		return "", nil, werr
	}
	for i, op := range ops {
		if i > 0 {
			if _, werr := f.WriteString("\n"); werr != nil {
				return bail(werr)
			}
		}
		if _, werr := f.WriteString(op.NFT()); werr != nil {
			return bail(werr)
		}
	}
	if _, werr := f.WriteString("\n"); werr != nil {
		return bail(werr)
	}
	if cerr := f.Close(); cerr != nil {
		_ = os.Remove(f.Name())
		return "", nil, cerr
	}
	path = f.Name()
	cleanup = func() { _ = os.Remove(path) }
	return path, cleanup, nil
}

// archive copies src into auditDir as audit-YYYYMMDD-HHMMSS.nft.
// The destination directory is created if missing.
func archive(src, auditDir string) (string, error) {
	if err := os.MkdirAll(auditDir, 0o750); err != nil {
		return "", fmt.Errorf("create audit dir: %w", err)
	}
	// Millisecond resolution: two commits inside the same second can't
	// collide on the audit filename. (O_EXCL would otherwise fail the
	// later commit.)
	dst := filepath.Join(auditDir,
		fmt.Sprintf("audit-%s.nft", time.Now().Format("20060102-150405.000")))
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
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = os.Remove(dst)
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
