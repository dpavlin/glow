package utils

import (
	"strings"
	"testing"

	"charm.land/glamour/v2"
	xansi "github.com/charmbracelet/x/ansi"
)

const wideTable = `# T

| A | B |
| --- | --- |
| aaaaaaaaaa bbbbbbbbbb cccccccccc dddddddddd eeeeeeeeee ffffffffff | 12345 67890 abcdefghij klmnopqrst uvwxyz0123 |
`

// renderWideTable renders a table whose natural width far exceeds the 40-column
// word wrap, and reports the widest line in *display cells*, the line count and
// whether anything was truncated with an ellipsis.
//
// Measuring in cells matters: the box-drawing characters glamour emits are three
// bytes each, so a byte-based measurement reports ~110 for a line that is only
// 38 cells wide.
func renderWideTable(t *testing.T, wrap bool, width uint) (maxCells, lines int, ellipsis bool) {
	t.Helper()

	opts := append([]glamour.TermRendererOption{
		GlamourStyle("dark", false),
		glamour.WithWordWrap(40),
	}, TableOptions(wrap, width)...)

	r, err := glamour.NewTermRenderer(opts...)
	if err != nil {
		t.Fatalf("new renderer: %v", err)
	}
	out, err := r.Render(wideTable)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	plain := xansi.Strip(out)
	for _, l := range strings.Split(plain, "\n") {
		if w := xansi.StringWidth(l); w > maxCells {
			maxCells = w
		}
		lines++
	}
	return maxCells, lines, strings.Contains(plain, "…")
}

// The default combination must not pass WithTableWidth(0): glamour reads that as
// "unconstrained", which silently turns wrapped tables into unwrapped ones.
func TestTableOptionsDefaultKeepsUpstreamWrapping(t *testing.T) {
	if got := TableOptions(true, 0); len(got) != 0 {
		t.Errorf("TableOptions(true, 0) should add no options so glamour keeps its "+
			"default wrap-to-render-width behaviour, got %d options", len(got))
	}

	maxCells, lines, ellipsis := renderWideTable(t, true, 0)
	if maxCells > 40 {
		t.Errorf("expected cells wrapped to the 40-column render width, got %d cells", maxCells)
	}
	if lines < 10 {
		t.Errorf("expected wrapping to spread the table over more lines, got %d", lines)
	}
	if ellipsis {
		t.Error("wrapping must not truncate with an ellipsis")
	}
}

func TestTableOptionsNaturalWidth(t *testing.T) {
	maxCells, lines, ellipsis := renderWideTable(t, false, 0)
	if maxCells < 100 {
		t.Errorf("expected the table at natural content width (>100 cells), got %d", maxCells)
	}
	if lines > 8 {
		t.Errorf("expected the table to stay on its own lines, got %d", lines)
	}
	if ellipsis {
		t.Error("natural width must not truncate with an ellipsis")
	}
}

func TestTableOptionsExplicitWidth(t *testing.T) {
	// An explicit width constrains the table well below its natural width.
	maxCells, _, _ := renderWideTable(t, true, 60)
	if maxCells > 62 {
		t.Errorf("expected the table constrained to ~60 columns, got %d cells", maxCells)
	}

	// With wrapping off the same width truncates instead.
	maxCells, _, ellipsis := renderWideTable(t, false, 60)
	if maxCells > 62 {
		t.Errorf("expected the table constrained to ~60 columns, got %d cells", maxCells)
	}
	if !ellipsis {
		t.Error("wrap=false at a fixed width should truncate overflowing cells")
	}
}
