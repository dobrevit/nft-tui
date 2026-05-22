package ui

import (
	"fmt"
	"strings"
)

// sparkSamples is the per-rule ring-buffer length. At the default 2 s
// refresh that's 2 minutes of history — long enough to spot bursts and
// short enough to render on an 80-column line.
const sparkSamples = 60

// sparkBlocks indexes into the Unicode bar set. Index 0 is whitespace
// (no sample / zero rate); 1..8 fill progressively. Held as a []rune
// so we can index by 1/8th of the max sample without recomputing UTF-8
// offsets.
var sparkBlocks = []rune(" ▁▂▃▄▅▆▇█")

// sparkline renders samples as a unicode bar plot of the same length.
// All samples are normalised to the maximum value in the slice; an
// all-zero slice renders as blanks.
//
// Returns a plain-ASCII-or-block string; the caller embeds it into a
// tview TextView. The padded-blank case keeps fixed-width layouts
// stable when traffic stops.
func sparkline(samples []float64) string {
	if len(samples) == 0 {
		return ""
	}
	var maxv float64
	for _, s := range samples {
		if s > maxv {
			maxv = s
		}
	}
	if maxv == 0 {
		return strings.Repeat(" ", len(samples))
	}
	var b strings.Builder
	b.Grow(len(samples) * 4) // worst-case 3-byte block + slack
	for _, s := range samples {
		idx := int(s / maxv * float64(len(sparkBlocks)-1))
		if idx >= len(sparkBlocks) {
			idx = len(sparkBlocks) - 1
		}
		if idx < 0 {
			idx = 0
		}
		b.WriteRune(sparkBlocks[idx])
	}
	return b.String()
}

// pushSample appends v to buf, keeping at most cap samples. Returns
// the (possibly trimmed) result. Used as the ring-buffer primitive on
// the explorer's per-rule sample maps.
func pushSample(buf []float64, v float64, cap int) []float64 {
	buf = append(buf, v)
	if len(buf) > cap {
		buf = buf[len(buf)-cap:]
	}
	return buf
}

// formatSparkline returns the full UI label including the max value
// scale on the left, so the operator can read "max 8.4K pps" alongside
// the bars. Falls back to an empty-state hint when buf is empty.
func formatSparkline(buf []float64, unit string) string {
	if len(buf) == 0 {
		return "[gray](no samples yet — refresh a few times)[-]"
	}
	var maxv float64
	for _, s := range buf {
		if s > maxv {
			maxv = s
		}
	}
	return fmt.Sprintf("max %s\n%s", humanRate(maxv, unit), sparkline(buf))
}
