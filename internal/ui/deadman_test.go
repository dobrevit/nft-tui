package ui

import (
	"strings"
	"testing"
	"time"
)

// TestCountdownBar verifies the visual countdown shrinks proportionally
// and changes colour as the deadline approaches.
func TestCountdownBar(t *testing.T) {
	const total = 60 * time.Second
	full := countdownBar(total, total)
	if !strings.Contains(full, "[green]") {
		t.Errorf("full bar should be green, got %q", full)
	}
	if strings.Count(full, "█") != 40 {
		t.Errorf("full bar should be 40 filled blocks, got %d in %q",
			strings.Count(full, "█"), full)
	}
	if strings.Count(full, "░") != 0 {
		t.Errorf("full bar should have no empty blocks, got %d", strings.Count(full, "░"))
	}

	half := countdownBar(30*time.Second, total)
	if strings.Count(half, "█") != 20 || strings.Count(half, "░") != 20 {
		t.Errorf("half-full bar should be 20+20, got %d filled, %d empty in %q",
			strings.Count(half, "█"), strings.Count(half, "░"), half)
	}

	low := countdownBar(10*time.Second, total)
	if !strings.Contains(low, "[yellow]") {
		t.Errorf("10s remaining should be yellow, got %q", low)
	}

	critical := countdownBar(3*time.Second, total)
	if !strings.Contains(critical, "[red]") {
		t.Errorf("3s remaining should be red, got %q", critical)
	}

	empty := countdownBar(0, total)
	if strings.Count(empty, "█") != 0 {
		t.Errorf("empty bar should have no filled blocks, got %q", empty)
	}
}

// TestCountdownBarClampsOutOfRange ensures values past either bound
// don't panic or render with negative widths.
func TestCountdownBarClampsOutOfRange(t *testing.T) {
	// Remaining > total: clamp filled to width.
	got := countdownBar(120*time.Second, 60*time.Second)
	if strings.Count(got, "█") != 40 {
		t.Errorf("over-full clamp: got %d filled blocks, want 40", strings.Count(got, "█"))
	}

	// Negative remaining: clamp filled to 0.
	got = countdownBar(-5*time.Second, 60*time.Second)
	if strings.Count(got, "█") != 0 {
		t.Errorf("negative remaining: got %d filled blocks, want 0", strings.Count(got, "█"))
	}

	// Zero total: empty string (avoid div-by-zero).
	if got := countdownBar(time.Second, 0); got != "" {
		t.Errorf("zero total: got %q, want empty", got)
	}
}
