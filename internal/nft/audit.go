package nft

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"
)

// auditFilename is the rolling log under AuditDir. Append-only,
// human-readable, grep-friendly. Each entry is a `#`-prefixed header
// line plus an optional payload (the nft script for commits).
const auditFilename = "audit.log"

// AuditEntry describes one operation worth recording.
type AuditEntry struct {
	// Action is "commit", "restore", or "rollback". Free-form to allow
	// future verbs without an enum migration.
	Action string

	// Operator identifies who ran the operation. UID is always set;
	// User is the resolved login name (best effort — may be empty if
	// /etc/passwd lookup failed).
	UID  int
	User string

	// Ops is the staged-op count for a commit; ignored for restore /
	// rollback.
	Ops int

	// File is the per-commit archive filename (relative is fine), set
	// by Commit. For restore / rollback this is the snapshot source.
	File string

	// Body is the full nft script applied — only set for commits.
	// Restores / rollbacks reference the source file instead so the
	// audit log doesn't balloon with every rule.
	Body string

	// At is the timestamp. Zero means "now"; callers normally leave
	// it zero so the file's mtime ordering matches the entries.
	At time.Time
}

// format produces the on-disk representation: one `#`-prefixed header
// line, then the body (if any), then a blank line separator. Designed
// to round-trip through `grep "^# " audit.log` for a one-line summary
// of every operation.
func (e AuditEntry) format() string {
	at := e.At
	if at.IsZero() {
		at = time.Now()
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# %s  uid=%d", at.UTC().Format(time.RFC3339), e.UID)
	if e.User != "" {
		fmt.Fprintf(&b, " (%s)", e.User)
	}
	fmt.Fprintf(&b, "  action=%s", e.Action)
	if e.Ops > 0 {
		fmt.Fprintf(&b, "  ops=%d", e.Ops)
	}
	if e.File != "" {
		fmt.Fprintf(&b, "  file=%s", e.File)
	}
	b.WriteByte('\n')
	if e.Body != "" {
		b.WriteString(e.Body)
		if !strings.HasSuffix(e.Body, "\n") {
			b.WriteByte('\n')
		}
	}
	b.WriteByte('\n')
	return b.String()
}

// Audit appends e to <AuditDir>/audit.log. AuditDir is created if
// missing. The operation is best-effort: errors are returned so the
// caller can surface them, but a missing AuditDir is silently skipped.
//
// The file is opened with O_APPEND|O_CREATE, so concurrent writes
// from multiple operator sessions interleave cleanly (POSIX guarantees
// O_APPEND atomicity up to PIPE_BUF; one entry is well within that).
func (c *Committer) Audit(e AuditEntry) error {
	if c.AuditDir == "" {
		return nil
	}
	if err := os.MkdirAll(c.AuditDir, 0o750); err != nil {
		return fmt.Errorf("create audit dir: %w", err)
	}
	path := filepath.Join(c.AuditDir, auditFilename)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open audit log: %w", err)
	}
	defer func() { _ = f.Close() }()
	if e.UID == 0 && e.User == "" {
		e.UID, e.User = currentOperator()
	}
	if _, err := f.WriteString(e.format()); err != nil {
		return fmt.Errorf("write audit entry: %w", err)
	}
	return nil
}

// currentOperator returns the UID and resolved username of the running
// process. Username is best-effort — falls back to empty if the user
// db is unavailable (container with no /etc/passwd, for instance).
func currentOperator() (uid int, name string) {
	uid = os.Getuid()
	if u, err := user.Current(); err == nil {
		name = u.Username
	}
	return uid, name
}
