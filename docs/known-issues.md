# Known issues — `feat/unwrapped-tables`

Found by reviewing and then *measuring* the branch against `origin/main`
(`7b2431d`). Reproduce with
[`../scripts/local-checks.sh`](../scripts/local-checks.sh).

**Status legend:** ✅ fixed on this branch · ⚠️ needs your decision · ❓ nit, open

| # | Severity | Issue | Status |
|---|---|---|---|
| 1 | 🔴 | glamour dependency is a local `replace` | ⚠️ fork pushed, PR pending |
| 2 | 🔴 | `--table-wrap` was a no-op; default silently unwrapped | ✅ fixed |
| 3 | 🔴 | config file overrode CLI flags | ✅ fixed |
| 4 | 🔴 | search highlights used byte offsets, not cell offsets | ✅ fixed |
| 5 | 🟠 | 120-column cap removed; prose spans the terminal | ⚠️ needs decision |
| 6 | 🟠 | CLI and TUI disagreed on table options | ✅ fixed (shared helper) |
| 7 | 🟠 | `GLOW_TABLE_WRAP` / `GLOW_TABLE_WIDTH` were dead | ✅ fixed (viper.BindEnv) |
| 8 | 🟠 | `gofmt` failed on changed files | ✅ fixed |
| 9 | 🟠 | stray `test_table.md` at repo root | ✅ moved to `testdata/` |
| 10 | 🟠 | no user-facing docs for the new flags/keys | ⚠️ open |
| 11 | 🟡 | Enter could jump back to the top of the document | ✅ fixed |
| 12 | 🟡 | status bar rendered `0/N` for a `-1` index | ✅ fixed |
| 13 | 🟡 | current match selected by value, marking duplicates | ✅ fixed |
| 14 | 🟡 | regex recompiled on every render | ✅ fixed (cached) |
| 15 | 🟡 | help overlay padding hard-coded | ❓ open |
| 16 | 🟡 | duplicated `searching \|\| filtering` checks | ✅ fixed (`inputActive()`) |
| 17 | 🟡 | `xansi.Strip` per line per render | ❓ open |
| 18 | 🟡 | `isTerminal` unused in the width branch | ❓ open (harmless) |
| 19 | 🔴 | **TUI did not wrap prose** — rendered before the window size was known | ✅ fixed |

---

## ⚠️ #1 — The glamour dependency is a local `replace`

```
go.mod:5:  replace charm.land/glamour/v2 => ../glamour
```

`glamour.WithTableWrap` / `WithTableWidth` live in
`../glamour @ 233e531`, now pushed to
[`dpavlin/glamour@feat/unwrapped-tables`](https://github.com/dpavlin/glamour).

Still to do: open the glamour PR, then drop the `replace`. Interim reproducible
build:

```bash
go mod edit -replace charm.land/glamour/v2=github.com/dpavlin/glamour@feat/unwrapped-tables
go mod tidy
```

---

## ✅ #2 — `--table-wrap` was a no-op

**Was** — `viper.IsSet("tableWidth")` is true even when unset (bound pflag), so
`WithTableWidth(0)` was always passed and glamour read that as *unconstrained*.
Every combination rendered unwrapped, including `--table-wrap=true`, while the
help text claimed the opposite.

**Now** — `main.go` delegates to `utils.TableOptions(wrap, width)`
(`utils/table.go`), which never passes `WithTableWidth(0)` when wrapping is on:

| invocation (`--width 40`) | before | after |
|---|---|---|
| default | 116 (unwrapped) | **38 (wrapped, same as base)** |
| `--table-wrap=true` | 116 | **38** |
| `--table-wrap=false` | 116 | 116 (natural width) |
| `--table-width 60` | 62 | 62 |

Covered by `utils.TestTableOptionsDefaultKeepsUpstreamWrapping`,
`TestTableOptionsNaturalWidth`, `TestTableOptionsExplicitWidth`.
Reproduce: `./scripts/local-checks.sh tables`

---

## ✅ #3 — Config file overrode CLI flags

**Was** — `if viper.IsSet("table_wrap") { tableWrap = viper.GetBool("table_wrap") }`
assigned last, so a `table_wrap: false` in `glow.yml` beat `--table-wrap=true`.

**Now** — `resolveTableSettings(changed func(string) bool)` gates each snake_case
fallback on the flag's `Changed` state, restoring flag > env > config > default.

| with `table_wrap: false` in the config | before | after |
|---|---|---|
| no flags | 116 | 116 (config applies) |
| `--table-wrap=true` | 116 (flag ignored) | **38 (flag wins)** |
| `--table-width 60` | 116 (flag ignored) | **62** |

Covered by `TestResolveTableSettingsPrecedence`.
Reproduce: `./scripts/local-checks.sh config`

---

## ✅ #4 — Byte offsets instead of cell offsets

**Was** — `re.FindAllStringIndex` returns byte offsets; `lipgloss.StyleRanges`
cuts with `ansi.Cut`, which indexes by display cell. Every match following a
multi-byte rune landed in the wrong place — and glamour table rows are full of
3-byte `│`.

```
"│ café │ naïve match here │"   byte 21-26 → highlighted "here "
"│ 你好 match │"                byte 11-16 → highlighted "h │"
```

**Now** — `findMatches()` converts to cells and accumulates the width as it walks
the matches, so it stays linear per line rather than quadratic in matches.
`submitSearch` and `recomputeSearchMatches` both use it.

Covered by `TestFindMatchesUsesCellOffsets` (box drawing, accents, CJK, emoji,
multiple matches per line), `TestSearchSurvivesReRender`.
Reproduce: `./scripts/local-checks.sh search`

---

## ⚠️ #5 — The 120-column cap is gone

`detectTerminalWidth()` replaced the old `if width > 120 { width = 120 }`, so
prose now spans the whole terminal:

```
COLUMNS=80    base=78   dev=78
COLUMNS=150   base=78   dev=148
COLUMNS=200   base=78   dev=198
```

Honouring `$COLUMNS` is a real improvement (base never did). The cap removal is
a separate judgement call that affects every reader, not just table users —
**your call**:

- keep it as is and document it, or
- cap `width` at 120 and let only tables run wide (`--table-wrap=false` already
  does that independently of the render width).

**Related gotcha (documented, not changed)** — if no `glow.yml` exists, glow
*writes* one with `width: 80` on first run, which disables auto-detection for
every later run. Always test width with an explicit `width: 0` config.

---

## ✅ #6 — CLI and TUI disagreed

The TUI used `else if cfg.TableWidth > 0`, the CLI used
`Changed("table-width") || viper.IsSet("tableWidth")`. Same config, two
renderings. Both now call `utils.TableOptions`, so CLI and TUI cannot drift.

---

## ✅ #7 — `GLOW_TABLE_WRAP` / `GLOW_TABLE_WIDTH` were dead

`runTUI` parsed the env into `ui.Config` and then overwrote both fields from
viper. Added `viper.BindEnv` for both keys in `init()`, so the env now sits
above the config file and below the flags.

```
GLOW_TABLE_WRAP=false   → 116 (natural width)
GLOW_TABLE_WRAP=true    → 38  (wrapped)
GLOW_TABLE_WIDTH=60     → 62
```

Reproduce: `./scripts/local-checks.sh config` (section 3b)

---

## ✅ #8 / #9 — Formatting and stray fixture

`gofmt -w` applied to `main.go`, `glow_test.go`, `ui/pager.go`, `ui/ui.go`,
`ui/styles.go`, `utils/table.go`. `gofmt -l` now reports only
`console_windows.go`, which is unformatted upstream and was left alone.

`test_table.md` → `testdata/table.md` (git-tracked rename).

---

## ⚠️ #10 — No user-facing documentation

`--table-wrap`, `--table-width`, `GLOW_TABLE_*` and the `/`, `&`, `n`/`N` keys
are still absent from `README.md` and the `--help` text beyond the flag
descriptions. Needed before an upstream PR.

---

## ✅ Nits 11–14, 16

| # | Fix |
|---|---|
| 11 | `firstMatchInView()` keeps the last match when everything is above the viewport instead of wrapping to the first |
| 12 | Status bar clamps the counter: `max(1, min(currentMatchIndex+1, len(matches)))` |
| 13 | `applyHighlights()` groups match **indices** per line and compares `idx == currentIdx`, so duplicates are not all marked current |
| 14 | `searchRE` / `filterRE` cached on the model; `setContent` and `recomputeSearchMatches` reuse them instead of recompiling |
| 16 | `pagerModel.inputActive()` replaces five copies of `state == searching \|\| state == filtering` |

Covered by `TestFirstMatchInView`, `TestApplyHighlightsSelectsCurrentMatchByIndex`,
`TestPagerSearchAndFilterIntegration` (which now also asserts the cached regexes
are cleared).

---

## ❓ Still open nits

| # | Where | Issue |
|---|---|---|
| 15 | `helpView` | 9 rows with hard-coded padding (`"                             " + col1[9]`); brittle in short terminals |
| 17 | `filterLines` / `findMatches` | `xansi.Strip` runs per line on every search/filter/re-render; caching stripped lines next to `fullLines` would help large documents |
| 18 | `validateOptions` | `isTerminal` is no longer used inside the width-detection branch (still used for the `notty` style, so it compiles) |

---

## 🔴 #19 — The TUI rendered before the window size was known, so nothing wrapped

**Symptom** — `glow file.md` wrapped prose correctly, `glow -t file.md` did not:
paragraphs ran past the right edge and the tail of each line was simply gone.

Measured on `ffzg-infra/procedures/new-wordpress-site.md` at 120 columns, before
the fix (real screen, via `tmux capture-pane`):

```
  4  120 |  Standard operating procedure for standing up a new department, ... on the  ffzg.unizg.hr
```

The source sentence ends with "suffix." — that word was **lost**, because the line
was clipped at the viewport edge rather than wrapped.

**Root cause** — two things compounded:

1. `ui.Init()` issued `renderWithGlamour()` immediately, before the first
   `WindowSizeMsg`. In `glamourRender` the wrap width is
   `min(GlamourMaxWidth, viewport.Width())`, and the viewport was still **0**
   wide, so glamour got `WithWordWrap(0)` — wrapping disabled.
2. The viewport has `SoftWrap == false` (so that unwrapped tables can scroll
   horizontally), which means those long prose lines were **clipped** instead of
   wrapped. Nothing re-rendered once the real size arrived: for a document opened
   by path, `pager.currentDocument.Body` was never populated, so the pager's own
   resize re-render rendered an empty string.

**Fix** — three parts:

- `ui.Init()` no longer renders. It only checks the file is readable. The
  comment in the code says why: the wrap width is baked into the rendered
  output, so rendering before the size is known bakes in zero.
- `ui.Update()` on `WindowSizeMsg` renders the document once the width is
  known, via a new `documentBody()` helper that reads and caches the body for
  documents opened by path (this is also what makes **resize** follow the new
  width).
- `glamourRender()` falls back to `common.width` when the viewport is still
  unsized, so no caller can accidentally disable wrapping again.

Verified after the fix (real screen):

```
120 cols: line 4 = 118 cells, line 5 = "suffix."        (wrapped, nothing lost)
 90 cols: line 4 =  87 cells, line 5 = "website on ..."
```

Covered by `ui/render_test.go`:
`TestGlamourRenderBeforeViewportIsSized` (fails without the fix: 215 cells),
`TestGlamourRenderUsesViewportWidth`, `TestWindowSizeRerendersLoadedDocument`.

---

## Tooling note

`task lint` cannot run here: the repo's `.golangci.yml` uses the golangci-lint
**v2** config format (`version: "2"`) while the installed binary is **1.56.1**,
which fails to parse it:

```
level=error msg="Can't read config: can't unmarshal config by viper: 'Version' expected a map, got 'string'"
```

Verified instead with `go vet ./...` (clean) and `gofmt -l` (clean except the
pre-existing `console_windows.go`). Install golangci-lint v2 to run the project's
own lint set.

---

## What was already solid

- `viewport.SetXOffset` usage; horizontal scrolling of unwrapped tables.
- Filter + search compose through `baseLines()`.
- `esc` layering (clear search → clear filter → leave document) matches `less`.
- `unload()` resets all search/filter state and both viewport offsets.
- `updateStyles` on `tea.BackgroundColorMsg`.
- Invalid regex falls back to `regexp.QuoteMeta`.
- `lipgloss.StyleRanges` is ANSI-aware, so highlights preserve glamour's styling.
