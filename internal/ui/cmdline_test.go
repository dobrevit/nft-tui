package ui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseCommand(t *testing.T) {
	cases := []struct {
		in      string
		kind    cmdKind
		arg     string
	}{
		{"", cmdSearch, ""},
		{"10.0.0.0", cmdSearch, "10.0.0.0"},
		{"w /tmp/snap.nft", cmdWrite, "/tmp/snap.nft"},
		{"write   /tmp/snap.nft", cmdWrite, "/tmp/snap.nft"},
		{"w", cmdWrite, ""},      // verb alone — UI prompts for path
		{"r /tmp/snap.nft", cmdRead, "/tmp/snap.nft"},
		{"restore /tmp/snap.nft", cmdRead, "/tmp/snap.nft"},
		{"read /tmp/snap.nft", cmdRead, "/tmp/snap.nft"},
		// Words that start with a verb letter must not be matched.
		{"wonderful", cmdSearch, "wonderful"},
		{"reload", cmdSearch, "reload"},
		// Leading/trailing whitespace tolerated.
		{"  w /tmp/snap.nft  ", cmdWrite, "/tmp/snap.nft"},
	}
	for _, c := range cases {
		got := parseCommand(c.in)
		if got.kind != c.kind || got.arg != c.arg {
			t.Errorf("parseCommand(%q) = %+v, want kind=%v arg=%q",
				c.in, got, c.kind, c.arg)
		}
	}
}

func TestExpandTilde(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("UserHomeDir unavailable in this environment")
	}
	cases := []struct{ in, want string }{
		{"~", home},
		{"~/foo.nft", filepath.Join(home, "foo.nft")},
		{"~/dir/sub.nft", filepath.Join(home, "dir/sub.nft")},
		{"/abs/path.nft", "/abs/path.nft"},
		{"relative.nft", "relative.nft"},
		// A bare path starting with ~something (no slash) is NOT expanded:
		// ~user-style expansion is too magical for a single-user tool.
		{"~user/foo", "~user/foo"},
	}
	for _, c := range cases {
		if got := expandTilde(c.in); got != c.want {
			t.Errorf("expandTilde(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
