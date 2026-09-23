# Pager search and line filter

`less`-style search (`/`) and line filtering (`&`) added to the document viewer in
`d6a20b8`, plus horizontal scrolling fixes from `9508d26`.

All code lives in `ui/pager.go` unless noted.

## Keys

| key | mode | effect |
|---|---|---|
| `/` | browse | enter **search** mode: prompt `/`, input cleared, focused |
| `&` | browse | enter **filter** mode: prompt `&`, input cleared, focused |
| `Enter` | search / filter | submit (`submitSearch` / `submitFilter`), return to browse |
| `Esc` | search / filter | abort — input discarded, back to browse |
| `Ctrl+C` | search / filter | abort (same as `Esc`) |
| `n` | browse | next match, wraps around |
| `N` | browse | previous match, wraps around |
| `Esc` | browse | clear search → (if none) clear filter → (if none) handled by `ui.go`: unload document |
| `q` | browse | quit; in any other pager state it just resets to browse |
| `h` / `←`, `l` / `→` | browse | horizontal scroll (handled by the bubbles viewport) |
| `?` | browse | help overlay (now lists `/`, `n/N`, `&`) |

`&` was also wired into the stash browser so `/` and `&` both open its filter
(`ui/stash.go:532`).

The `left` / `h` / `delete` handler in `ui/ui.go` that used to unload the
document was removed in `9508d26`, which is what frees `h`/`l` for horizontal
scrolling.

## Semantics

### Pattern compilation (`compileSearchRegex`, `ui/pager.go:837`)

- **Smart case**: if the pattern contains any uppercase letter it is used as-is;
  otherwise it is prefixed with `(?i)`. Same rule as `less`.
- **Regex with a literal fallback**: the pattern is compiled as a Go regexp; if
  that fails it is recompiled with `regexp.QuoteMeta`, so `foo(bar` searches for
  the literal string instead of erroring out.
- Empty pattern → `nil` (no search).

### Search (`submitSearch`, `ui/pager.go:362`)

- Matches are computed against `baseLines()`, which is the **filtered** line set
  when a filter is active, otherwise the full rendered document.
- Matching runs on `xansi.Strip(line)`, so ANSI styling is ignored when looking
  for the pattern.
- The current match is chosen as the first match at or below the viewport's
  current `YOffset`, falling back to index `0`.
- No matches → status message `Pattern not found: <pattern>`, highlights cleared.
- Submitting an empty pattern reuses the previous `searchPattern` if there is
  one, otherwise clears the search.

### Filter (`submitFilter`, `ui/pager.go:412`)

- Keeps whole rendered lines whose stripped text matches the pattern (styling of
  surviving lines is preserved).
- Applied to `fullLines`, so it is re-evaluated on every re-render (resize,
  reload, edit).
- No surviving lines → status message `No matching lines: <pattern>`, filter not
  applied.
- Empty pattern clears the filter.

### Composing the two

`baseLines()` is the single source of truth:

```go
func (m *pagerModel) baseLines() []string {
    if m.isFiltered() { return m.filteredLines }
    return m.fullLines
}
```

So `&error` then `/timeout` searches only within the error lines. Clearing the
filter (`Esc`) recomputes matches against the full document
(`clearFilter` → `recomputeSearchMatches`).

### Scrolling to a match (`scrollToCurrentMatch`, `ui/pager.go:508`)

- Vertical: only moves if the match is off-screen; positions it one third down.
- Horizontal: if the match is outside the visible columns, `SetXOffset(max(0,
  colStart-4))` so the match appears near the left edge with 4 columns of
  context. This is what makes search usable on the unwrapped tables.

### Status bar formats

While typing, the bar shows the logo plus the live input field. Otherwise:

| state | note |
|---|---|
| filter + matches | `<doc> • &<filter> • [i/n] /<pattern>` |
| filter only | `<doc> • &<filter> (N lines)` |
| matches only | `<doc> • [i/n] /<pattern>` |
| neither | `<doc>` |

### Lifecycle

`unload()` clears `fullLines`, `filteredLines`, `filterPattern`,
`searchMatches`, `searchPattern`, `currentMatchIndex`, and both viewport
offsets, so a reopened document never inherits a stale filter.

`updateStyles` (called on `tea.BackgroundColorMsg`) re-applies highlights so
the search survives a terminal theme change.

## Highlight rendering

`applyHighlights` (`ui/pager.go:807`) groups matches per line and applies
`lipgloss.StyleRanges` with:

- `styles.highlightStyle` — yellow-green background (`ui/styles.go:164`)
- `styles.selectedHighlightStyle` — fuchsia background + bold, for the current
  match (`ui/styles.go:168`)

`lipgloss.StyleRanges` is ANSI-aware (it cuts with `ansi.Cut`), so glamour's own
styling survives highlighting. **But** it indexes by *display cell*, and the
branch feeds it *byte* offsets — see
[known-issues.md #4](known-issues.md#4-search-highlights-use-byte-offsets-not-cell-offsets).

## Known rough edges

1. Byte-vs-cell offsets → wrong highlight position on any non-ASCII line.
2. `submitSearch` falls back to match index `0` when nothing is at/below the
   viewport, so Enter can yank you back to the top of the document.
3. The status bar prints `currentMatchIndex+1`, which renders `0/N` when the
   index is `-1`.
4. `applyHighlights` identifies the selected match by struct equality
   (`matches[currentIdx] == m`), so duplicate matches all render as selected.
5. The help overlay grew from 6 to 9 rows with hard-coded padding
   (`"                             " + col1[9]`); it is brittle in short
   terminals.
