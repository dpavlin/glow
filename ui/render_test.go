package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	xansi "github.com/charmbracelet/x/ansi"
)

// A single long paragraph: if wrapping is disabled it comes back as one very
// wide line whose tail is lost when the viewport clips it.
const wrapProse = "Standard operating procedure for standing up a new department, conference, or " +
	"project website on the faculty suffix, which is fully automated by the provisioning " +
	"script and needs no manual certificate work at all."

func maxCells(s string) int {
	m := 0
	for _, l := range strings.Split(xansi.Strip(s), "\n") {
		if w := xansi.StringWidth(l); w > m {
			m = w
		}
	}
	return m
}

func renderProse(t *testing.T, viewportWidth, commonWidth, maxWidth int) string {
	t.Helper()

	common := &commonModel{
		styles: newStyles(true),
		width:  commonWidth,
		height: 40,
		cfg: Config{
			GlamourEnabled:  true,
			GlamourStyle:    "dark",
			GlamourMaxWidth: uint(maxWidth),
			TableWrap:       true,
		},
	}

	m := newPagerModel(common)
	if viewportWidth > 0 {
		m.setSize(viewportWidth, 40)
	}
	m.currentDocument = markdown{Body: wrapProse, Note: "doc.md"}

	out, err := glamourRender(m, wrapProse)
	if err != nil {
		t.Fatalf("glamourRender: %v", err)
	}
	return out
}

// Init() renders the document before the first WindowSizeMsg arrives, so the
// viewport is still zero wide at that point. glamourRender has to fall back to
// the terminal width it does know; otherwise the word wrap is 0, wrapping is
// disabled, and the viewport (which does not soft-wrap) clips the line instead.
func TestGlamourRenderBeforeViewportIsSized(t *testing.T) {
	out := renderProse(t, 0, 120, 120)

	if got := maxCells(out); got > 120 {
		t.Errorf("unsized viewport: expected prose wrapped to 120 cells, got %d", got)
	}
	if !strings.Contains(xansi.Strip(out), "at all.") {
		t.Error("unsized viewport: the end of the paragraph was lost")
	}
}

func TestGlamourRenderUsesViewportWidth(t *testing.T) {
	out := renderProse(t, 60, 120, 120)

	if got := maxCells(out); got > 60 {
		t.Errorf("expected prose wrapped to the 60-cell viewport, got %d", got)
	}
	if !strings.Contains(xansi.Strip(out), "at all.") {
		t.Error("the end of the paragraph was lost")
	}
}

// The wrap width is baked into the rendered output, so a resize must trigger a
// re-render; otherwise the document keeps whatever wrap it was first drawn with.
func TestWindowSizeRerendersLoadedDocument(t *testing.T) {
	cfg := Config{
		GlamourEnabled:  true,
		GlamourStyle:    "dark",
		GlamourMaxWidth: 120,
		TableWrap:       true,
	}

	m := newModel(cfg, wrapProse)
	doc, ok := m.(model)
	if !ok {
		t.Fatalf("newModel returned %T, want model", m)
	}
	if doc.state != stateShowDocument {
		t.Fatalf("expected stateShowDocument, got %v", doc.state)
	}

	updated, cmd := doc.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if cmd == nil {
		t.Fatal("expected a command when the window size arrives with a document loaded")
	}
	if !schedulesRender(cmd) {
		t.Error("expected the window size to schedule a re-render of the loaded document")
	}

	got, ok := updated.(model)
	if !ok {
		t.Fatalf("Update returned %T, want model", updated)
	}
	if got.common.width != 120 {
		t.Errorf("expected the common width to be updated to 120, got %d", got.common.width)
	}
}

// schedulesRender reports whether running cmd yields (possibly via tea.Batch) a
// contentRenderedMsg, i.e. a glamour re-render.
func schedulesRender(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	msg := cmd()
	if _, ok := msg.(contentRenderedMsg); ok {
		return true
	}
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		return false
	}
	for _, c := range batch {
		if schedulesRender(c) {
			return true
		}
	}
	return false
}

func TestCodeFilePreservesFullWidthAndScrollsByDefault(t *testing.T) {
	wideCode := `$recipient = Get-Recipient -Filter "EmailAddresses -like '*@domain.com'" | Select-Object DisplayName, PrimarySmtpAddress, RecipientTypeDetails, WhenCreated`

	common := &commonModel{
		styles: newStyles(true),
		width:  60,
		height: 40,
		cfg: Config{
			GlamourEnabled: true,
			GlamourStyle:   "dark",
		},
	}

	p := newPagerModel(common)
	p.setSize(60, 40)
	p.currentDocument = markdown{
		Body: wideCode,
		Note: "lookup_recipient.ps1",
	}

	out, err := glamourRender(p, wideCode)
	if err != nil {
		t.Fatalf("glamourRender: %v", err)
	}

	// The rendered output must contain the complete code line, not truncated to 60 columns.
	if !strings.Contains(out, "WhenCreated") {
		t.Fatal("expected full code line to be preserved, but 'WhenCreated' was truncated")
	}

	p.setContent(out)

	// By default, SoftWrap is false so lines are not folded and horizontal scroll is enabled
	if p.viewport.SoftWrap {
		t.Fatal("expected SoftWrap to be false by default")
	}

	if p.viewport.HorizontalScrollPercent() > 0.0 {
		t.Errorf("initial horizontal scroll percent should be 0, got %f", p.viewport.HorizontalScrollPercent())
	}

	// Scroll right
	p.viewport.ScrollRight(10)
	if p.viewport.XOffset() <= 0 {
		t.Errorf("expected XOffset > 0 after scrolling right on wide code file, got %d", p.viewport.XOffset())
	}

	// Line numbers should be rendered via LeftGutterFunc
	if p.viewport.LeftGutterFunc == nil {
		t.Fatal("expected LeftGutterFunc to be set for code file")
	}
	view := p.viewport.View()
	if !strings.Contains(view, "1") {
		t.Errorf("expected line number 1 in viewport view, got %q", view)
	}
}

func TestPagerToggleFoldingWithSKey(t *testing.T) {
	wideCode := `$recipient = Get-Recipient -Filter "EmailAddresses -like '*@domain.com'" | Select-Object DisplayName, PrimarySmtpAddress, RecipientTypeDetails, WhenCreated`

	common := &commonModel{
		styles: newStyles(true),
		width:  60,
		height: 40,
		cfg: Config{
			GlamourEnabled: true,
			GlamourStyle:   "dark",
		},
	}

	p := newPagerModel(common)
	p.setSize(60, 40)
	p.currentDocument = markdown{
		Body: wideCode,
		Note: "lookup_recipient.ps1",
	}

	out, err := glamourRender(p, wideCode)
	if err != nil {
		t.Fatalf("glamourRender: %v", err)
	}
	p.setContent(out)

	if p.viewport.SoftWrap {
		t.Fatal("expected SoftWrap to be false initially")
	}

	// Press S to toggle folding on
	p, cmd := p.update(tea.KeyPressMsg{Code: 'S', Text: "S"})
	if cmd == nil {
		t.Fatal("expected command on S toggle")
	}
	if !p.viewport.SoftWrap {
		t.Fatal("expected SoftWrap to be true after pressing S")
	}

	// With soft wrap (fold), visible lines should be split across multiple rows so end of line is visible
	view := p.viewport.View()
	if !strings.Contains(view, "WhenCreated") {
		t.Errorf("expected 'WhenCreated' to be visible in folded view, got %q", view)
	}

	// Press S again to toggle back to unfolded / horizontal scrolling
	p, _ = p.update(tea.KeyPressMsg{Code: 'S', Text: "S"})
	if p.viewport.SoftWrap {
		t.Fatal("expected SoftWrap to be false after pressing S second time")
	}
}
