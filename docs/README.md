# Local documentation — `feat/unwrapped-tables`

These notes live in the working tree only. They document what this branch does,
how to reproduce it, and what is still open. Nothing here is intended for
upstream as-is.

Branch: `feat/unwrapped-tables` (3 commits on top of `origin/main` `7b2431d`,
plus the fix-up pass described below)

```
d6a20b8 feat(ui): add '/' search and '&' line filter like less in pager document viewer
9508d26 fix(ui): allow left/right and h/l keys to scroll horizontally in document view
3dabc93 feat: auto-detect terminal width and support unwrapped tables
```

~700 lines across `main.go`, `ui/pager.go`, `ui/ui.go`, `ui/styles.go`,
`ui/config.go`, `ui/stash.go`, plus tests. The fix pass adds
`utils/table.go` and ~350 lines of tests.

## Contents

| File | What's in it |
|---|---|
| [`tables.md`](tables.md) | The unwrapped-table feature: flags, config keys, env vars, and the glamour option semantics underneath it |
| [`pager-search-filter.md`](pager-search-filter.md) | `/` search, `&` line filter, `n`/`N`, `esc` layering, horizontal scrolling, status bar formats |
| [`known-issues.md`](known-issues.md) | Every defect found in review, its status, and what fixed it |
| [`dev-setup.md`](dev-setup.md) | The sibling `../glamour` dependency, the `replace` directive, build/test commands, and what must change before a PR |

## Reproducing everything

```bash
./scripts/local-checks.sh            # build branch + base, run all four check groups
./scripts/local-checks.sh tables     # table width/wrap matrix
./scripts/local-checks.sh width      # terminal width detection
./scripts/local-checks.sh config     # config/env vs CLI-flag precedence
./scripts/local-checks.sh search     # search offset + selection tests
```

The script never touches `~/.config/glow/glow.yml`; it builds throwaway HOMEs
under `/tmp/glow-local-checks`.

## Headline results (measured, `--width 40`, two very wide cells)

| invocation | before | after |
|---|---|---|
| default | 116 (unwrapped) | **38 (wrapped, same as base)** |
| `--table-wrap=true` | 116 | **38** |
| `--table-wrap=false` | 116 | 116 (natural width) |
| `--table-width 60` | 62 | 62 |
| base `7b2431d` | 38 | — |

## What the fix pass changed

| Area | Change |
|---|---|
| **TUI wrapping** | The TUI rendered before the window size was known, so glamour got `WithWordWrap(0)` and long lines were clipped by the (non-soft-wrapping) viewport. `Init()` no longer renders; the first `WindowSizeMsg` does, via a new `documentBody()` helper that caches the body for documents opened by path. Resize now follows the new width |
| Table options | New `utils.TableOptions(wrap, width)` used by **both** the CLI and the TUI; `WithTableWidth(0)` is no longer sent when wrapping is on |
| Precedence | `resolveTableSettings(changed)` — CLI flag now beats `table_wrap` / `table_width` in the config file |
| Env | `viper.BindEnv` wired up, so `GLOW_TABLE_WRAP` / `GLOW_TABLE_WIDTH` actually work |
| Search offsets | `findMatches()` converts byte offsets to **display cells** (box-drawing, accents, CJK, emoji) and stays linear per line |
| Match selection | `applyHighlights` groups match **indices** per line, so duplicate matches are not all marked current |
| Navigation | `firstMatchInView` keeps the last match instead of jumping to the top of the document |
| Status bar | counter clamped, no more `0/N` |
| Regex | compiled once and cached on the model (`searchRE` / `filterRE`) |
| Duplication | `pagerModel.inputActive()` replaces five copies of the searching/filtering check |
| Housekeeping | `gofmt` clean; `test_table.md` → `testdata/table.md`; flag help text corrected |

## Status

- ✅ `glow -t file.md` now wraps prose like the CLI does, at any terminal width.
- ✅ `--table-wrap` / `--table-width` behave as documented, in CLI and TUI alike.
- ✅ Config no longer overrides the command line; env vars work.
- ✅ Search highlights land on the right text for non-ASCII content.
- ⚠️ `go.mod` still carries `replace charm.land/glamour/v2 => ../glamour`.
  The glamour work is pushed to
  [dpavlin/glamour@feat/unwrapped-tables](https://github.com/dpavlin/glamour)
  and needs a PR before the `replace` can be dropped — that is the critical path.
- ⚠️ The removed 120-column cap means prose spans the whole terminal. Kept as is;
  see [known-issues.md #5](known-issues.md) for the trade-off.
- ⚠️ `README.md` still needs the new flags, env vars and keys documented.

## Forks

Created and pushed (both on `feat/unwrapped-tables`, remote name `dpavlin`):

- [dpavlin/glow](https://github.com/dpavlin/glow)
- [dpavlin/glamour](https://github.com/dpavlin/glamour)
