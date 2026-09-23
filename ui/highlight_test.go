package ui

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/viewport"
	"charm.land/lipgloss/v2"
	xansi "github.com/charmbracelet/x/ansi"
)

// findMatches must return display-cell coordinates, because that is the index
// space lipgloss.StyleRanges cuts in. Asserting via xansi.Cut on the stripped
// line is the round-trip check: if the offsets were byte- or rune-based, the cut
// would come back with the wrong text for any line containing multi-byte runes.
func TestFindMatchesUsesCellOffsets(t *testing.T) {
	cases := []struct {
		name  string
		line  string
		pat   string
		count int
	}{
		{
			name:  "ascii only",
			line:  "plain ascii match here",
			pat:   "match",
			count: 1,
		},
		{
			name:  "box drawing and accents (glamour table row)",
			line:  "│ café │ naïve match here │",
			pat:   "match",
			count: 1,
		},
		{
			name:  "wide cjk runes",
			line:  "│ 你好 match │",
			pat:   "match",
			count: 1,
		},
		{
			name:  "emoji",
			line:  "🚀 rocket match end",
			pat:   "match",
			count: 1,
		},
		{
			name:  "multiple matches after multi-byte runes",
			line:  "│ café match │ naïve match │",
			pat:   "match",
			count: 2,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			re := compileSearchRegex(tc.pat)
			if re == nil {
				t.Fatalf("compileSearchRegex(%q) returned nil", tc.pat)
			}

			matches := findMatches(re, []string{tc.line})
			if len(matches) != tc.count {
				t.Fatalf("expected %d matches, got %d: %+v", tc.count, len(matches), matches)
			}

			stripped := xansi.Strip(tc.line)
			for _, m := range matches {
				got := xansi.Cut(stripped, m.colStart, m.colEnd)
				if got != tc.pat {
					t.Errorf("cell offsets (%d,%d) cut %q, want %q (line %q)",
						m.colStart, m.colEnd, got, tc.pat, stripped)
				}
			}
		})
	}
}

// The current match is identified by its index in the match slice. Identifying it
// by value would highlight every duplicate occurrence as the current one.
func TestApplyHighlightsSelectsCurrentMatchByIndex(t *testing.T) {
	line := "dup dup dup"
	re := compileSearchRegex("dup")
	matches := findMatches(re, []string{line})
	if len(matches) != 3 {
		t.Fatalf("expected 3 matches, got %d", len(matches))
	}

	hlStyle := lipgloss.NewStyle().Background(lipgloss.Color("#00ff00"))
	selStyle := lipgloss.NewStyle().Background(lipgloss.Color("#ff00ff"))
	hlSeq := sgrPrefix(hlStyle)
	selSeq := sgrPrefix(selStyle)

	out := applyHighlights([]string{line}, matches, 1, hlStyle, selStyle)
	got := strings.Join(out, "\n")

	if n := strings.Count(got, selSeq); n != 1 {
		t.Errorf("expected exactly 1 selected highlight, got %d in %q", n, got)
	}
	if n := strings.Count(got, hlSeq); n != 2 {
		t.Errorf("expected exactly 2 plain highlights, got %d in %q", n, got)
	}
	// The selected one must be the second occurrence.
	selAt := strings.Index(got, selSeq)
	hlAt := strings.Index(got, hlSeq)
	if selAt < hlAt {
		t.Errorf("selected highlight should come after the first plain one: %q", got)
	}
}

// sgrPrefix returns the escape sequence a style emits ahead of its content, so
// rendered output can be searched for that style.
func sgrPrefix(s lipgloss.Style) string {
	rendered := s.Render("X")
	i := strings.Index(rendered, "X")
	if i < 0 {
		return ""
	}
	return rendered[:i]
}

func TestFirstMatchInView(t *testing.T) {
	matches := []searchMatch{{line: 2}, {line: 5}, {line: 9}}

	if got := firstMatchInView(matches, 0); got != 0 {
		t.Errorf("yOff 0: got %d, want 0", got)
	}
	if got := firstMatchInView(matches, 5); got != 1 {
		t.Errorf("yOff 5: got %d, want 1", got)
	}
	if got := firstMatchInView(matches, 6); got != 2 {
		t.Errorf("yOff 6: got %d, want 2", got)
	}
	// Everything above the viewport: keep the last match rather than wrapping to
	// the top of the document.
	if got := firstMatchInView(matches, 20); got != 2 {
		t.Errorf("yOff 20 (past every match): got %d, want 2 (last match, not the first)", got)
	}
}

func TestSearchHighlightFlow(t *testing.T) {
	lines := []string{
		"\x1b[1m# Header\x1b[0m",
		"This is a \x1b[32mtest\x1b[0m line with test again.",
		"Another line without matches.",
		"Final test line.",
	}

	matches := findMatches(compileSearchRegex("test"), lines)
	if len(matches) != 3 {
		t.Fatalf("expected 3 matches, got %d: %+v", len(matches), matches)
	}

	hlStyle := lipgloss.NewStyle().Background(lipgloss.Color("11")).Foreground(lipgloss.Color("0"))
	selStyle := lipgloss.NewStyle().Background(lipgloss.Color("13")).Foreground(lipgloss.Color("0"))

	highlighted := applyHighlights(lines, matches, 0, hlStyle, selStyle)
	vp := viewport.New(viewport.WithWidth(80), viewport.WithHeight(10))
	vp.SetContentLines(highlighted)
	t.Logf("View with match 0 selected:\n%s", vp.View())

	highlighted1 := applyHighlights(lines, matches, 1, hlStyle, selStyle)
	vp.SetContentLines(highlighted1)
	t.Logf("View with match 1 selected:\n%s", vp.View())

	// Highlighting must not change the visible text.
	for i := range lines {
		if a, b := xansi.Strip(lines[i]), xansi.Strip(highlighted1[i]); a != b {
			t.Errorf("highlighting changed visible text on line %d:\n got %q\nwant %q", i, b, a)
		}
	}
}

func TestLineFilter(t *testing.T) {
	lines := []string{
		"apple pie",
		"banana split",
		"apple tart",
		"cherry cake",
	}

	re := compileSearchRegex("apple")
	if re == nil {
		t.Fatal("expected valid regex")
	}

	filtered := filterLines(re, lines)
	if len(filtered) != 2 {
		t.Fatalf("expected 2 filtered lines, got %d: %v", len(filtered), filtered)
	}
	if filtered[0] != "apple pie" || filtered[1] != "apple tart" {
		t.Fatalf("unexpected filtered lines: %v", filtered)
	}
}

// The filter must ignore styling when matching, and keep the styled line intact.
func TestLineFilterIgnoresAnsi(t *testing.T) {
	lines := []string{
		"\x1b[31merror\x1b[0m: boom",
		"\x1b[32mfine\x1b[0m",
	}
	filtered := filterLines(compileSearchRegex("error"), lines)
	if len(filtered) != 1 {
		t.Fatalf("expected 1 filtered line, got %d: %v", len(filtered), filtered)
	}
	if filtered[0] != lines[0] {
		t.Fatalf("filter should preserve the original styled line, got %q", filtered[0])
	}
}

func TestPagerSearchAndFilterIntegration(t *testing.T) {
	common := &commonModel{
		styles: newStyles(true),
		width:  80,
		height: 24,
	}

	p := newPagerModel(common)
	p.setSize(80, 24)
	doc := "First line\nSecond line with keyword\nThird line\nFourth line with keyword too\nFifth line"
	p.setContent(doc)

	// Test line filter with &
	p.promptInput.SetValue("keyword")
	p, _ = p.submitFilter()

	if !p.isFiltered() {
		t.Fatal("expected pager to be in filtered state")
	}
	if len(p.baseLines()) != 2 {
		t.Fatalf("expected 2 filtered lines, got %d: %v", len(p.baseLines()), p.baseLines())
	}

	// Test search within filtered content
	p.promptInput.SetValue("too")
	p, _ = p.submitSearch()

	if len(p.searchMatches) != 1 {
		t.Fatalf("expected 1 match for 'too' in filtered lines, got %d", len(p.searchMatches))
	}

	// Clear search
	p.clearSearch()
	if p.hasSearchHighlights() {
		t.Fatal("expected search highlights to be cleared")
	}
	if p.searchRE != nil {
		t.Fatal("expected cached search regex to be cleared")
	}

	// Clear filter
	p.clearFilter()
	if p.isFiltered() {
		t.Fatal("expected filter to be cleared")
	}
	if p.filterRE != nil {
		t.Fatal("expected cached filter regex to be cleared")
	}
	if len(p.baseLines()) != 5 {
		t.Fatalf("expected 5 full lines, got %d: %v", len(p.baseLines()), p.baseLines())
	}
}

// A search must survive a re-render (which is what resize/reload do) and stay
// anchored to the same cells.
func TestSearchSurvivesReRender(t *testing.T) {
	common := &commonModel{styles: newStyles(true), width: 80, height: 24}
	p := newPagerModel(common)
	p.setSize(80, 24)
	p.setContent("│ café │ naïve match here │\nsecond line")

	p.promptInput.SetValue("match")
	p, _ = p.submitSearch()
	if len(p.searchMatches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(p.searchMatches))
	}
	before := p.searchMatches[0]

	// Simulate a re-render of the same content.
	p.setContent("│ café │ naïve match here │\nsecond line")
	after := p.searchMatches[0]

	if before != after {
		t.Fatalf("match moved across re-render: %+v -> %+v", before, after)
	}
	if got := xansi.Cut("│ café │ naïve match here │", after.colStart, after.colEnd); got != "match" {
		t.Fatalf("after re-render the offsets point at %q, want %q", got, "match")
	}
}

func TestCompileSearchRegexSmartCase(t *testing.T) {
	if re := compileSearchRegex("foo"); re == nil || !re.MatchString("FOO") {
		t.Error("lowercase pattern should match case-insensitively")
	}
	if re := compileSearchRegex("Foo"); re == nil || re.MatchString("foo") {
		t.Error("pattern with an uppercase letter should be case-sensitive")
	}
	// Invalid regex must fall back to a literal match rather than fail outright.
	if re := compileSearchRegex("a(b"); re == nil || !re.MatchString("a(b") {
		t.Error("invalid regex should fall back to a literal match")
	}
	if re := compileSearchRegex(""); re != nil {
		t.Error("empty pattern should compile to nil")
	}
}
