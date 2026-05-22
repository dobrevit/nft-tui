package ui

import (
	"encoding/base64"
	"fmt"
	"io"
	"os"
)

// emitOSC52 writes an OSC 52 escape sequence to w so the controlling
// terminal copies s to the system clipboard. Works over SSH on
// terminals that honour OSC 52 (iTerm2, kitty, alacritty, foot,
// wezterm; some require explicit enablement). Falls back silently if
// the terminal ignores the sequence — there is no in-band confirmation.
//
// Format: ESC ] 52 ; c ; <base64(s)> BEL
//
// "c" selects the clipboard buffer (vs primary selection "p").
func emitOSC52(w io.Writer, s string) error {
	enc := base64.StdEncoding.EncodeToString([]byte(s))
	_, err := fmt.Fprintf(w, "\x1b]52;c;%s\x07", enc)
	return err
}

// yankToTerminal copies s to the clipboard via OSC 52, writing to
// /dev/tty so the sequence reaches the terminal even when stdout is
// piped or captured.
func yankToTerminal(s string) error {
	tty, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0)
	if err != nil {
		// Fall back to stdout — better than nothing.
		return emitOSC52(os.Stdout, s)
	}
	defer tty.Close()
	return emitOSC52(tty, s)
}
