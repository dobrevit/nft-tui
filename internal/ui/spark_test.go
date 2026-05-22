package ui

import (
	"strings"
	"testing"
)

func TestSparklineEmpty(t *testing.T) {
	if got := sparkline(nil); got != "" {
		t.Errorf("sparkline(nil) = %q, want empty", got)
	}
}

func TestSparklineAllZero(t *testing.T) {
	got := sparkline([]float64{0, 0, 0, 0})
	if got != "    " {
		t.Errorf("all-zero should render as blanks, got %q", got)
	}
}

func TestSparklineScales(t *testing.T) {
	// max == 8, samples should map to indices 0..8 in sparkBlocks.
	got := sparkline([]float64{0, 1, 2, 3, 4, 5, 6, 7, 8})
	if []rune(got)[0] != ' ' {
		t.Errorf("zero sample should be blank; got %q", string([]rune(got)[0]))
	}
	if []rune(got)[8] != '█' {
		t.Errorf("max sample should be full block; got %q", string([]rune(got)[8]))
	}
}

func TestPushSampleTrims(t *testing.T) {
	var buf []float64
	for i := 0; i < 100; i++ {
		buf = pushSample(buf, float64(i), 10)
	}
	if len(buf) != 10 {
		t.Errorf("buffer length = %d, want 10", len(buf))
	}
	// Should contain the most recent 10 values: 90..99.
	if buf[0] != 90 || buf[9] != 99 {
		t.Errorf("ring buffer kept the wrong slice: %v", buf)
	}
}

func TestFormatSparklineHasScale(t *testing.T) {
	got := formatSparkline([]float64{0, 50, 100, 50, 0}, "pps")
	if !strings.Contains(got, "max") {
		t.Errorf("missing `max` line in: %q", got)
	}
	if !strings.Contains(got, "pps") {
		t.Errorf("missing unit suffix in: %q", got)
	}
}

func TestFormatSparklineEmpty(t *testing.T) {
	if got := formatSparkline(nil, "pps"); !strings.Contains(got, "no samples") {
		t.Errorf("empty buffer should show a hint; got %q", got)
	}
}
