package ui

import (
	"fmt"
	"math"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/glamour/v2"
	"charm.land/glow/v3/utils"
	"charm.land/lipgloss/v2"
	"github.com/atotto/clipboard"
	"github.com/charmbracelet/log"
	xansi "github.com/charmbracelet/x/ansi"
	"github.com/fsnotify/fsnotify"
	runewidth "github.com/mattn/go-runewidth"
	"github.com/muesli/reflow/ansi"
	"github.com/muesli/reflow/truncate"
	"github.com/muesli/termenv"
)

const (
	statusBarHeight = 1
	lineNumberWidth = 4
)

var pagerHelpHeight int

type (
	contentRenderedMsg string
	reloadMsg          struct{}
)

type pagerState int

const (
	pagerStateBrowse pagerState = iota
	pagerStateStatusMessage
	pagerStateSearching
	pagerStateFiltering
)

type searchMatch struct {
	line     int
	colStart int
	colEnd   int
}

type pagerModel struct {
	common   *commonModel
	viewport viewport.Model
	state    pagerState
	showHelp bool

	statusMessage      string
	statusMessageTimer *time.Timer

	// Search and filter input
	promptInput textinput.Model

	// Unfiltered lines rendered by Glamour
	fullLines []string

	// Filter state
	filterPattern string
	filteredLines []string

	// Search state
	searchPattern     string
	searchMatches     []searchMatch
	currentMatchIndex int

	// Current document being rendered, sans-glamour rendering. We cache
	// it here so we can re-render it on resize.
	currentDocument markdown

	watcher *fsnotify.Watcher
}

func newPagerModel(common *commonModel) pagerModel {
	// Init viewport
	vp := viewport.New()

	promptInput := textinput.New()
	promptInput.Prompt = "/"
	promptInput.SetVirtualCursor(true)
	tsi := promptInput.Styles()
	tsi.Focused.Prompt = common.styles.stashInputPromptStyle
	tsi.Blurred.Prompt = common.styles.stashInputPromptStyle
	tsi.Cursor.Color = common.styles.fuchsia
	promptInput.SetStyles(tsi)

	m := pagerModel{
		common:            common,
		state:             pagerStateBrowse,
		viewport:          vp,
		promptInput:       promptInput,
		currentMatchIndex: -1,
	}
	m.initWatcher()
	return m
}

func (m *pagerModel) updateStyles(styles Styles) {
	tsi := m.promptInput.Styles()
	tsi.Focused.Prompt = styles.stashInputPromptStyle
	tsi.Blurred.Prompt = styles.stashInputPromptStyle
	tsi.Cursor.Color = styles.fuchsia
	m.promptInput.SetStyles(tsi)
	if m.hasSearchHighlights() {
		m.updateDisplayedContent()
	}
}

func (m *pagerModel) setSize(w, h int) {
	m.viewport.SetWidth(w)
	m.viewport.SetHeight(h - statusBarHeight)

	if m.showHelp {
		if pagerHelpHeight == 0 {
			pagerHelpHeight = strings.Count(m.helpView(), "\n")
		}
		m.viewport.SetHeight(m.viewport.Height() - (statusBarHeight + pagerHelpHeight))
	}
}

func (m *pagerModel) isFiltered() bool {
	return m.filterPattern != ""
}

func (m *pagerModel) hasSearchHighlights() bool {
	return len(m.searchMatches) > 0
}

func (m *pagerModel) baseLines() []string {
	if m.isFiltered() {
		return m.filteredLines
	}
	return m.fullLines
}

func (m *pagerModel) updateDisplayedContent() {
	base := m.baseLines()
	if m.hasSearchHighlights() {
		highlighted := applyHighlights(base, m.searchMatches, m.currentMatchIndex, m.common.styles.highlightStyle, m.common.styles.selectedHighlightStyle)
		m.viewport.SetContentLines(highlighted)
	} else {
		m.viewport.SetContentLines(base)
	}
}

func (m *pagerModel) setContent(s string) {
	m.fullLines = strings.Split(s, "\n")
	if m.isFiltered() {
		re := compileSearchRegex(m.filterPattern)
		if re != nil {
			var filtered []string
			for _, line := range m.fullLines {
				if re.MatchString(xansi.Strip(line)) {
					filtered = append(filtered, line)
				}
			}
			m.filteredLines = filtered
		}
	}
	if m.searchPattern != "" {
		m.recomputeSearchMatches()
	}
	m.updateDisplayedContent()
}

func (m *pagerModel) toggleHelp() {
	m.showHelp = !m.showHelp
	m.setSize(m.common.width, m.common.height)
	if m.viewport.PastBottom() {
		m.viewport.GotoBottom()
	}
}

type pagerStatusMessage struct {
	message string
	isError bool
}

// Perform stuff that needs to happen after a successful markdown stash. Note
// that the returned command should be sent back the through the pager
// update function.
func (m *pagerModel) showStatusMessage(msg pagerStatusMessage) tea.Cmd {
	// Show a success message to the user
	m.state = pagerStateStatusMessage
	m.statusMessage = msg.message
	if m.statusMessageTimer != nil {
		m.statusMessageTimer.Stop()
	}
	m.statusMessageTimer = time.NewTimer(statusMessageTimeout)

	return waitForStatusMessageTimeout(pagerContext, m.statusMessageTimer)
}

func (m *pagerModel) unload() {
	log.Debug("unload")
	if m.showHelp {
		m.toggleHelp()
	}
	if m.statusMessageTimer != nil {
		m.statusMessageTimer.Stop()
	}
	m.state = pagerStateBrowse
	m.fullLines = nil
	m.filteredLines = nil
	m.filterPattern = ""
	m.searchMatches = nil
	m.searchPattern = ""
	m.currentMatchIndex = -1
	m.viewport.SetContent("")
	m.viewport.SetYOffset(0)
	m.viewport.SetXOffset(0)
	m.unwatchFile()
}

func (m pagerModel) update(msg tea.Msg) (pagerModel, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if m.state == pagerStateSearching || m.state == pagerStateFiltering {
			switch msg.String() {
			case keyEsc, "ctrl+c":
				m.state = pagerStateBrowse
				return m, nil
			case keyEnter:
				if m.state == pagerStateSearching {
					return m.submitSearch()
				}
				return m.submitFilter()
			default:
				m.promptInput, cmd = m.promptInput.Update(msg)
				return m, cmd
			}
		}

		switch msg.String() {
		case "q":
			if m.state != pagerStateBrowse {
				m.state = pagerStateBrowse
				return m, nil
			}

		case keyEsc:
			if m.state != pagerStateBrowse {
				m.state = pagerStateBrowse
				return m, nil
			}
			if m.hasSearchHighlights() {
				m.clearSearch()
				return m, nil
			}
			if m.isFiltered() {
				m.clearFilter()
				return m, nil
			}

		case "/":
			m.state = pagerStateSearching
			m.promptInput.Prompt = "/"
			m.promptInput.SetValue("")
			m.promptInput.Focus()
			return m, textinput.Blink

		case "&":
			m.state = pagerStateFiltering
			m.promptInput.Prompt = "&"
			m.promptInput.SetValue("")
			m.promptInput.Focus()
			return m, textinput.Blink

		case "n":
			m.nextMatch()
			return m, nil

		case "N":
			m.prevMatch()
			return m, nil

		case "home", "g":
			m.viewport.GotoTop()
		case "end", "G":
			m.viewport.GotoBottom()

		case "d":
			m.viewport.HalfPageDown()

		case "u":
			m.viewport.HalfPageUp()

		case "e":
			lineno := int(math.RoundToEven(float64(m.viewport.TotalLineCount()) * m.viewport.ScrollPercent()))
			if m.viewport.AtTop() {
				lineno = 0
			}
			log.Info(
				"opening editor",
				"file", m.currentDocument.localPath,
				"line", fmt.Sprintf("%d/%d", lineno, m.viewport.TotalLineCount()),
			)
			return m, openEditor(m.currentDocument.localPath, lineno)

		case "c":
			// Copy using OSC 52
			termenv.Copy(m.currentDocument.Body)
			// Copy using native system clipboard
			_ = clipboard.WriteAll(m.currentDocument.Body)
			cmds = append(cmds, m.showStatusMessage(pagerStatusMessage{"Copied contents", false}))

		case "r":
			return m, loadLocalMarkdown(&m.currentDocument)

		case "?":
			m.toggleHelp()
		}

	// Glow has rendered the content
	case contentRenderedMsg:
		log.Info("content rendered", "state", m.state)

		m.setContent(string(msg))
		cmds = append(cmds, m.watchFile)

	// The file was changed on disk and we're reloading it
	case reloadMsg:
		return m, loadLocalMarkdown(&m.currentDocument)

	// We've finished editing the document, potentially making changes. Let's
	// retrieve the latest version of the document so that we display
	// up-to-date contents.
	case editorFinishedMsg:
		return m, loadLocalMarkdown(&m.currentDocument)

	// We've received terminal dimensions, either for the first time or
	// after a resize
	case tea.WindowSizeMsg:
		return m, renderWithGlamour(m, m.currentDocument.Body)

	case statusMessageTimeoutMsg:
		m.state = pagerStateBrowse
	}

	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m *pagerModel) submitSearch() (pagerModel, tea.Cmd) {
	pattern := strings.TrimSpace(m.promptInput.Value())
	if pattern == "" {
		if m.searchPattern != "" {
			pattern = m.searchPattern
		} else {
			m.clearSearch()
			m.state = pagerStateBrowse
			return *m, nil
		}
	}

	re := compileSearchRegex(pattern)
	if re == nil {
		m.state = pagerStateBrowse
		return *m, nil
	}

	m.searchPattern = pattern
	base := m.baseLines()
	var matches []searchMatch
	for i, line := range base {
		stripped := xansi.Strip(line)
		locs := re.FindAllStringIndex(stripped, -1)
		for _, loc := range locs {
			matches = append(matches, searchMatch{line: i, colStart: loc[0], colEnd: loc[1]})
		}
	}

	if len(matches) == 0 {
		m.clearSearch()
		m.state = pagerStateBrowse
		return *m, m.showStatusMessage(pagerStatusMessage{fmt.Sprintf("Pattern not found: %s", pattern), false})
	}

	m.searchMatches = matches
	matchIdx := 0
	for idx, match := range matches {
		if match.line >= m.viewport.YOffset() {
			matchIdx = idx
			break
		}
	}
	m.currentMatchIndex = matchIdx
	m.updateDisplayedContent()
	m.scrollToCurrentMatch()
	m.state = pagerStateBrowse
	return *m, nil
}

func (m *pagerModel) submitFilter() (pagerModel, tea.Cmd) {
	pattern := strings.TrimSpace(m.promptInput.Value())
	if pattern == "" {
		m.clearFilter()
		m.state = pagerStateBrowse
		return *m, nil
	}

	re := compileSearchRegex(pattern)
	if re == nil {
		m.state = pagerStateBrowse
		return *m, nil
	}

	var filtered []string
	for _, line := range m.fullLines {
		if re.MatchString(xansi.Strip(line)) {
			filtered = append(filtered, line)
		}
	}

	if len(filtered) == 0 {
		m.state = pagerStateBrowse
		return *m, m.showStatusMessage(pagerStatusMessage{fmt.Sprintf("No matching lines: %s", pattern), false})
	}

	m.filterPattern = pattern
	m.filteredLines = filtered
	m.recomputeSearchMatches()
	m.updateDisplayedContent()
	m.viewport.GotoTop()
	m.state = pagerStateBrowse
	return *m, nil
}

func (m *pagerModel) recomputeSearchMatches() {
	if m.searchPattern == "" {
		m.searchMatches = nil
		m.currentMatchIndex = -1
		return
	}
	re := compileSearchRegex(m.searchPattern)
	if re == nil {
		m.searchMatches = nil
		m.currentMatchIndex = -1
		return
	}
	base := m.baseLines()
	var matches []searchMatch
	for i, line := range base {
		stripped := xansi.Strip(line)
		locs := re.FindAllStringIndex(stripped, -1)
		for _, loc := range locs {
			matches = append(matches, searchMatch{line: i, colStart: loc[0], colEnd: loc[1]})
		}
	}
	m.searchMatches = matches
	if len(matches) == 0 {
		m.currentMatchIndex = -1
	} else if m.currentMatchIndex >= len(matches) || m.currentMatchIndex < 0 {
		m.currentMatchIndex = 0
	}
}

func (m *pagerModel) clearSearch() {
	m.searchPattern = ""
	m.searchMatches = nil
	m.currentMatchIndex = -1
	m.updateDisplayedContent()
}

func (m *pagerModel) clearFilter() {
	m.filterPattern = ""
	m.filteredLines = nil
	m.recomputeSearchMatches()
	m.updateDisplayedContent()
}

func (m *pagerModel) nextMatch() {
	if len(m.searchMatches) == 0 {
		return
	}
	m.currentMatchIndex = (m.currentMatchIndex + 1) % len(m.searchMatches)
	m.updateDisplayedContent()
	m.scrollToCurrentMatch()
}

func (m *pagerModel) prevMatch() {
	if len(m.searchMatches) == 0 {
		return
	}
	m.currentMatchIndex = (m.currentMatchIndex - 1 + len(m.searchMatches)) % len(m.searchMatches)
	m.updateDisplayedContent()
	m.scrollToCurrentMatch()
}

func (m *pagerModel) scrollToCurrentMatch() {
	if m.currentMatchIndex < 0 || m.currentMatchIndex >= len(m.searchMatches) {
		return
	}
	match := m.searchMatches[m.currentMatchIndex]
	vpHeight := max(1, m.viewport.Height())
	yOff := m.viewport.YOffset()
	if match.line < yOff || match.line >= yOff+vpHeight {
		targetY := max(0, match.line-vpHeight/3)
		m.viewport.SetYOffset(targetY)
	}

	vpWidth := max(1, m.viewport.Width())
	xOff := m.viewport.XOffset()
	if match.colEnd > xOff+vpWidth || match.colStart < xOff {
		m.viewport.SetXOffset(max(0, match.colStart-4))
	}
}

func (m pagerModel) View() string {
	var b strings.Builder
	fmt.Fprint(&b, m.viewport.View()+"\n")

	// Footer
	m.statusBarView(&b)

	if m.showHelp {
		fmt.Fprint(&b, "\n"+m.helpView())
	}

	return b.String()
}

func (m pagerModel) statusBarView(b *strings.Builder) {
	styles := m.common.styles
	logo := glowLogoView(m.common.styles)

	if m.state == pagerStateSearching || m.state == pagerStateFiltering {
		inputView := m.promptInput.View()
		inputWidth := ansi.PrintableRuneWidth(inputView)
		padding := max(0, m.common.width-ansi.PrintableRuneWidth(logo)-inputWidth-1)
		emptySpace := strings.Repeat(" ", padding)
		emptySpace = styles.statusBarNoteStyle(emptySpace)
		fmt.Fprintf(b, "%s %s%s", logo, inputView, emptySpace)
		return
	}

	const (
		minPercent               float64 = 0.0
		maxPercent               float64 = 1.0
		percentToStringMagnitude float64 = 100.0
	)

	showStatusMessage := m.state == pagerStateStatusMessage

	// Scroll percent
	percent := math.Max(minPercent, math.Min(maxPercent, m.viewport.ScrollPercent()))
	scrollPercent := fmt.Sprintf(" %3.f%% ", percent*percentToStringMagnitude)
	if showStatusMessage {
		scrollPercent = styles.statusBarMessageScrollPosStyle(scrollPercent)
	} else {
		scrollPercent = styles.statusBarScrollPosStyle(scrollPercent)
	}

	// "Help" note
	var helpNote string
	if showStatusMessage {
		helpNote = styles.statusBarMessageHelpStyle(" ? Help ")
	} else {
		helpNote = styles.statusBarHelpStyle(" ? Help ")
	}

	// Note
	var note string
	if showStatusMessage {
		note = m.statusMessage
	} else {
		note = m.currentDocument.Note
		if m.isFiltered() && len(m.searchMatches) > 0 {
			note = fmt.Sprintf("%s • &%s • [%d/%d] /%s", note, m.filterPattern, m.currentMatchIndex+1, len(m.searchMatches), m.searchPattern)
		} else if m.isFiltered() {
			note = fmt.Sprintf("%s • &%s (%d lines)", note, m.filterPattern, len(m.baseLines()))
		} else if len(m.searchMatches) > 0 {
			note = fmt.Sprintf("%s • [%d/%d] /%s", note, m.currentMatchIndex+1, len(m.searchMatches), m.searchPattern)
		}
	}
	note = truncate.StringWithTail(" "+note+" ", uint(max(0, //nolint:gosec
		m.common.width-
			ansi.PrintableRuneWidth(logo)-
			ansi.PrintableRuneWidth(scrollPercent)-
			ansi.PrintableRuneWidth(helpNote),
	)), ellipsis)
	if showStatusMessage {
		note = styles.statusBarMessageStyle(note)
	} else {
		note = styles.statusBarNoteStyle(note)
	}

	// Empty space
	padding := max(0,
		m.common.width-
			ansi.PrintableRuneWidth(logo)-
			ansi.PrintableRuneWidth(note)-
			ansi.PrintableRuneWidth(scrollPercent)-
			ansi.PrintableRuneWidth(helpNote),
	)
	emptySpace := strings.Repeat(" ", padding)
	if showStatusMessage {
		emptySpace = styles.statusBarMessageStyle(emptySpace)
	} else {
		emptySpace = styles.statusBarNoteStyle(emptySpace)
	}

	fmt.Fprintf(b, "%s%s%s%s%s",
		logo,
		note,
		emptySpace,
		scrollPercent,
		helpNote,
	)
}

func (m pagerModel) helpView() (s string) {
	col1 := []string{
		"/       search",
		"n/N     next/prev match",
		"&       filter lines",
		"g/home  go to top",
		"G/end   go to bottom",
		"c       copy contents",
		"e       edit this document",
		"r       reload this document",
		"esc     back to files / clear",
		"q       quit",
	}

	s += "\n"
	s += "k/↑      up                  " + col1[0] + "\n"
	s += "j/↓      down                " + col1[1] + "\n"
	s += "h/←      left                " + col1[2] + "\n"
	s += "l/→      right               " + col1[3] + "\n"
	s += "b/pgup   page up             " + col1[4] + "\n"
	s += "f/pgdn   page down           " + col1[5] + "\n"
	s += "u        ½ page up           " + col1[6] + "\n"
	s += "d        ½ page down         " + col1[7] + "\n"
	s += "                             " + col1[8] + "\n"
	s += "                             " + col1[9]

	s = indent(s, 2)

	// Fill up empty cells with spaces for background coloring
	if m.common.width > 0 {
		lines := strings.Split(s, "\n")
		for i := 0; i < len(lines); i++ {
			l := runewidth.StringWidth(lines[i])
			n := max(m.common.width-l, 0)
			lines[i] += strings.Repeat(" ", n)
		}

		s = strings.Join(lines, "\n")
	}

	return m.common.styles.helpViewStyle(s)
}

// COMMANDS

func renderWithGlamour(m pagerModel, md string) tea.Cmd {
	return func() tea.Msg {
		s, err := glamourRender(m, md)
		if err != nil {
			log.Error("error rendering with Glamour", "error", err)
			return errMsg{err}
		}
		return contentRenderedMsg(s)
	}
}

// This is where the magic happens.
func glamourRender(m pagerModel, markdown string) (string, error) {
	trunc := lipgloss.NewStyle().MaxWidth(m.viewport.Width() - lineNumberWidth).Render

	if !m.common.cfg.GlamourEnabled {
		return markdown, nil
	}

	isCode := !utils.IsMarkdownFile(m.currentDocument.Note)
	width := max(0, min(int(m.common.cfg.GlamourMaxWidth), m.viewport.Width())) //nolint:gosec
	if isCode {
		width = 0
	}

	options := []glamour.TermRendererOption{
		utils.GlamourStyle(m.common.cfg.GlamourStyle, isCode),
		glamour.WithWordWrap(width),
	}

	if m.common.cfg.PreserveNewLines {
		options = append(options, glamour.WithPreservedNewLines())
	}
	if !m.common.cfg.TableWrap {
		options = append(options, glamour.WithTableWrap(false), glamour.WithTableWidth(int(m.common.cfg.TableWidth)))
	} else if m.common.cfg.TableWidth > 0 {
		options = append(options, glamour.WithTableWidth(int(m.common.cfg.TableWidth)))
	}
	r, err := glamour.NewTermRenderer(options...)
	if err != nil {
		return "", fmt.Errorf("error creating glamour renderer: %w", err)
	}

	if isCode {
		markdown = utils.WrapCodeBlock(markdown, filepath.Ext(m.currentDocument.Note))
	}

	out, err := r.Render(markdown)
	if err != nil {
		return "", fmt.Errorf("error rendering markdown: %w", err)
	}

	if isCode {
		out = strings.TrimSpace(out)
	}

	// trim lines
	lines := strings.Split(out, "\n")

	var content strings.Builder
	for i, s := range lines {
		if isCode || m.common.cfg.ShowLineNumbers {
			content.WriteString(m.common.styles.lineNumberStyle(fmt.Sprintf("%"+fmt.Sprint(lineNumberWidth)+"d", i+1)))
			content.WriteString(trunc(s))
		} else {
			content.WriteString(s)
		}

		// don't add an artificial newline after the last split
		if i+1 < len(lines) {
			content.WriteRune('\n')
		}
	}

	return content.String(), nil
}

func (m *pagerModel) initWatcher() {
	var err error
	m.watcher, err = fsnotify.NewWatcher()
	if err != nil {
		log.Error("error creating fsnotify watcher", "error", err)
	}
}

func (m *pagerModel) watchFile() tea.Msg {
	dir := m.localDir()

	if err := m.watcher.Add(dir); err != nil {
		log.Error("error adding dir to fsnotify watcher", "error", err)
		return nil
	}

	log.Info("fsnotify watching dir", "dir", dir)

	for {
		select {
		case event, ok := <-m.watcher.Events:
			if !ok || event.Name != m.currentDocument.localPath {
				continue
			}

			if !event.Has(fsnotify.Write) && !event.Has(fsnotify.Create) {
				continue
			}

			log.Debug("fsnotify event", "file", event.Name, "event", event.Op)
			return reloadMsg{}
		case err, ok := <-m.watcher.Errors:
			if !ok {
				continue
			}
			log.Debug("fsnotify error", "dir", dir, "error", err)
		}
	}
}

func (m *pagerModel) unwatchFile() {
	dir := m.localDir()

	err := m.watcher.Remove(dir)
	if err == nil {
		log.Debug("fsnotify dir unwatched", "dir", dir)
	} else {
		log.Error("fsnotify fail to unwatch dir", "dir", dir, "error", err)
	}
}

func (m *pagerModel) localDir() string {
	return filepath.Dir(m.currentDocument.localPath)
}

func applyHighlights(lines []string, matches []searchMatch, currentIdx int, hlStyle, selStyle lipgloss.Style) []string {
	if len(matches) == 0 {
		return lines
	}
	res := make([]string, len(lines))
	copy(res, lines)

	lineMatches := make(map[int][]searchMatch)
	for _, m := range matches {
		lineMatches[m.line] = append(lineMatches[m.line], m)
	}

	for lineIdx, lMatches := range lineMatches {
		if lineIdx < 0 || lineIdx >= len(lines) {
			continue
		}
		ranges := make([]lipgloss.Range, 0, len(lMatches))
		for _, m := range lMatches {
			st := hlStyle
			if currentIdx >= 0 && currentIdx < len(matches) && matches[currentIdx] == m {
				st = selStyle
			}
			ranges = append(ranges, lipgloss.NewRange(m.colStart, m.colEnd, st))
		}
		res[lineIdx] = lipgloss.StyleRanges(lines[lineIdx], ranges...)
	}

	return res
}

func compileSearchRegex(pattern string) *regexp.Regexp {
	if pattern == "" {
		return nil
	}
	hasUpper := strings.ToLower(pattern) != pattern
	var expr string
	if hasUpper {
		expr = pattern
	} else {
		expr = "(?i)" + pattern
	}
	re, err := regexp.Compile(expr)
	if err != nil {
		if hasUpper {
			expr = regexp.QuoteMeta(pattern)
		} else {
			expr = "(?i)" + regexp.QuoteMeta(pattern)
		}
		re, err = regexp.Compile(expr)
		if err != nil {
			return nil
		}
	}
	return re
}
