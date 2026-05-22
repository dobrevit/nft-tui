package ui

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"
)

func TestEmitOSC52(t *testing.T) {
	var buf bytes.Buffer
	const payload = `tcp dport 22 accept`
	if err := emitOSC52(&buf, payload); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.HasPrefix(got, "\x1b]52;c;") {
		t.Errorf("missing OSC 52 prefix in %q", got)
	}
	if !strings.HasSuffix(got, "\x07") {
		t.Errorf("missing BEL terminator in %q", got)
	}
	encoded := strings.TrimSuffix(strings.TrimPrefix(got, "\x1b]52;c;"), "\x07")
	dec, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("payload not base64: %v", err)
	}
	if string(dec) != payload {
		t.Errorf("decoded payload = %q, want %q", dec, payload)
	}
}
