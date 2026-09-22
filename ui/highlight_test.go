package ui

import (
	"regexp"
	"testing"

	"charm.land/bubbles/v2/viewport"
	"charm.land/lipgloss/v2"
	xansi "github.com/charmbracelet/x/ansi"
)

func TestSearchHighlightFlow(t *testing.T) {
	lines := []string{
		"\x1b[1m# Header\x1b[0m",
		"This is a \x1b[32mtest\x1b[0m line with test again.",
		"Another line without matches.",
		"Final test line.",
	}

	re := regexp.MustCompile("(?i)test")
	var matches []searchMatch
	for i, line := range lines {
		stripped := xansi.Strip(line)
		locs := re.FindAllStringIndex(stripped, -1)
		for _, loc := range locs {
			matches = append(matches, searchMatch{line: i, colStart: loc[0], colEnd: loc[1]})
		}
	}

	if len(matches) != 3 {
		t.Fatalf("expected 3 matches, got %d", len(matches))
	}

	hlStyle := lipgloss.NewStyle().Background(lipgloss.Color("11")).Foreground(lipgloss.Color("0"))
	selStyle := lipgloss.NewStyle().Background(lipgloss.Color("13")).Foreground(lipgloss.Color("0"))

	// Highlight with match 0 selected
	highlighted := applyHighlights(lines, matches, 0, hlStyle, selStyle)

	vp := viewport.New(viewport.WithWidth(80), viewport.WithHeight(10))
	vp.SetContentLines(highlighted)

	v := vp.View()
	t.Logf("View with match 0 selected:\n%s", v)

	// Highlight with match 1 selected
	highlighted1 := applyHighlights(lines, matches, 1, hlStyle, selStyle)
	vp.SetContentLines(highlighted1)
	v1 := vp.View()
	t.Logf("View with match 1 selected:\n%s", v1)
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

	var filtered []string
	for _, line := range lines {
		if re.MatchString(xansi.Strip(line)) {
			filtered = append(filtered, line)
		}
	}

	if len(filtered) != 2 {
		t.Fatalf("expected 2 filtered lines, got %d: %v", len(filtered), filtered)
	}

	if filtered[0] != "apple pie" || filtered[1] != "apple tart" {
		t.Fatalf("unexpected filtered lines: %v", filtered)
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

	// Clear filter
	p.clearFilter()
	if p.isFiltered() {
		t.Fatal("expected filter to be cleared")
	}
	if len(p.baseLines()) != 5 {
		t.Fatalf("expected 5 full lines, got %d", len(p.baseLines()))
	}
}
