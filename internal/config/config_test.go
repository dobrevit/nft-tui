package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadParsesEveryField(t *testing.T) {
	p := writeConfig(t, `
refresh = "750ms"
write = true
monitor = false
theme = "high-contrast"
columns = "debug"
audit_dir = "/var/log/nft-tui"
`)
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Refresh.AsDuration() != 750*time.Millisecond {
		t.Errorf("refresh = %v, want 750ms", cfg.Refresh.AsDuration())
	}
	if cfg.Write == nil || *cfg.Write != true {
		t.Errorf("write = %v, want true", cfg.Write)
	}
	if cfg.Monitor == nil || *cfg.Monitor != false {
		t.Errorf("monitor = %v, want false", cfg.Monitor)
	}
	if cfg.Theme == nil || *cfg.Theme != "high-contrast" {
		t.Errorf("theme = %v, want high-contrast", cfg.Theme)
	}
	if cfg.Columns == nil || *cfg.Columns != "debug" {
		t.Errorf("columns = %v, want debug", cfg.Columns)
	}
	if cfg.AuditDir == nil || *cfg.AuditDir != "/var/log/nft-tui" {
		t.Errorf("audit_dir = %v, want /var/log/nft-tui", cfg.AuditDir)
	}
}

func TestLoadAbsenceIsNotError(t *testing.T) {
	p := filepath.Join(t.TempDir(), "missing.toml")
	cfg, err := Load(p)
	if err != nil {
		t.Errorf("missing config should not error; got %v", err)
	}
	if cfg.Refresh != nil || cfg.Write != nil {
		t.Errorf("missing config returned populated fields: %+v", cfg)
	}
}

func TestLoadEmptyPathIsNoOp(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Errorf("empty path should not error; got %v", err)
	}
	if cfg.Refresh != nil {
		t.Errorf("empty path returned populated cfg: %+v", cfg)
	}
}

func TestLoadRejectsUnknownKey(t *testing.T) {
	p := writeConfig(t, `chartreuse = "not a real field"`)
	_, err := Load(p)
	if err == nil {
		t.Fatal("expected error for unknown key, got nil")
	}
	if !strings.Contains(err.Error(), "chartreuse") {
		t.Errorf("error should name the unknown key; got %v", err)
	}
}

func TestLoadRejectsBadDuration(t *testing.T) {
	p := writeConfig(t, `refresh = "not-a-duration"`)
	_, err := Load(p)
	if err == nil {
		t.Fatal("expected error for malformed duration")
	}
}

func TestDefaultPathHonoursXDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/custom/cfg")
	if got, want := DefaultPath(), "/custom/cfg/nft-tui/config.toml"; got != want {
		t.Errorf("XDG_CONFIG_HOME path: got %q, want %q", got, want)
	}

	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "/home/operator")
	if got, want := DefaultPath(), "/home/operator/.config/nft-tui/config.toml"; got != want {
		t.Errorf("HOME fallback: got %q, want %q", got, want)
	}
}

func TestDurationAsDurationOnNilReceiver(t *testing.T) {
	// Guard against the convenience helper panicking on the zero-value
	// pointer — common code path is `cfg.Refresh.AsDuration()` even
	// when Refresh is nil.
	var d *Duration
	if got := d.AsDuration(); got != 0 {
		t.Errorf("nil receiver should yield 0; got %v", got)
	}
}
