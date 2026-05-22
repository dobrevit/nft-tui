// Package config loads the per-host nft-tui config file.
//
// Precedence (highest first):
//
//  1. Explicit CLI flags
//  2. Config file at $XDG_CONFIG_HOME/nft-tui/config.toml
//     (or $HOME/.config/nft-tui/config.toml if XDG_CONFIG_HOME is
//     unset; or --config <path> for an arbitrary location)
//  3. Built-in defaults baked into cmd/nft-tui/main.go
//
// Every field is a pointer so the caller can distinguish "set in
// config" from "absent". A bare value (e.g. refresh = 0s) would
// otherwise be indistinguishable from "not configured" and silently
// override the CLI default.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/BurntSushi/toml"
)

// Duration is a TOML-friendly wrapper over time.Duration that accepts
// human strings like "500ms", "2s", "1h30m". Without it the user
// would have to write nanoseconds (refresh = 2000000000) which is
// hostile.
type Duration time.Duration

// UnmarshalText implements encoding.TextUnmarshaler so BurntSushi/toml
// turns `refresh = "2s"` into a Duration during decode.
func (d *Duration) UnmarshalText(text []byte) error {
	v, err := time.ParseDuration(string(text))
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", text, err)
	}
	*d = Duration(v)
	return nil
}

// AsDuration returns the underlying time.Duration; convenience for
// callers that don't want the type cast at the use site.
func (d *Duration) AsDuration() time.Duration {
	if d == nil {
		return 0
	}
	return time.Duration(*d)
}

// Config mirrors the CLI flag set. Pointer types record "presence";
// nil means "no override, use the CLI default".
type Config struct {
	Refresh  *Duration `toml:"refresh"`
	Write    *bool     `toml:"write"`
	Monitor  *bool     `toml:"monitor"`
	Theme    *string   `toml:"theme"`
	Columns  *string   `toml:"columns"`
	AuditDir *string   `toml:"audit_dir"`
	LogFile  *string   `toml:"log_file"`
}

// Load reads the TOML file at path. A nonexistent path is not an
// error — the returned zero-value Config means "use defaults".
// Malformed content or other I/O failures ARE returned.
//
// An empty path returns the zero Config (no I/O attempted), useful
// when the caller wants to skip file loading entirely.
func Load(path string) (Config, error) {
	var cfg Config
	if path == "" {
		return cfg, nil
	}
	md, err := toml.DecodeFile(path, &cfg)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, fmt.Errorf("read %s: %w", path, err)
	}
	// Surface unknown keys early — a typo in the operator's config
	// would otherwise silently fall through to defaults, which is
	// surprising. (Reading a config and getting silently-different
	// behaviour is the worst kind of bug.)
	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		names := make([]string, len(undecoded))
		for i, k := range undecoded {
			names[i] = k.String()
		}
		return cfg, fmt.Errorf("unknown key(s) in %s: %v", path, names)
	}
	return cfg, nil
}

// DefaultPath returns $XDG_CONFIG_HOME/nft-tui/config.toml, falling
// back to $HOME/.config/nft-tui/config.toml. Returns "" if neither
// env var resolves (e.g. a chrooted runner with no /etc/passwd).
func DefaultPath() string {
	if d := os.Getenv("XDG_CONFIG_HOME"); d != "" {
		return filepath.Join(d, "nft-tui", "config.toml")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".config", "nft-tui", "config.toml")
}
