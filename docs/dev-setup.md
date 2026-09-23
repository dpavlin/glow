# Dev setup — glow + the local glamour fork

## Layout

```
~/go/glow        # this repo, branch feat/unwrapped-tables
~/go/glamour     # sibling checkout of https://github.com/charmbracelet/glamour
                 # branch feat/unwrapped-tables @ 233e531
```

Remotes in both repos:

| remote | URL | role |
|---|---|---|
| `origin` | `https://github.com/charmbracelet/{glow,glamour}.git` | upstream, read-only for us |
| `dpavlin` | `git@github.com:dpavlin/{glow,glamour}.git` | our forks, branches are pushed here |

Both feature branches are pushed and track `dpavlin/feat/unwrapped-tables`:

```bash
git push -u dpavlin feat/unwrapped-tables   # done for both repos
gh repo view dpavlin/glamour --json name,isFork,parent
```

`glow/go.mod:5` points the module at the sibling checkout:

```
replace charm.land/glamour/v2 => ../glamour
```

The replace is **relative**, so the two repos must stay siblings, and any copy of
`glow` elsewhere fails to build with:

```
replacement directory ../glamour does not exist
```

To build a copy somewhere else, rewrite the replace to an absolute path first:

```bash
sed -i 's|=> ../glamour|=> /home/dpavlin/go/glamour|' go.mod
```

## What the glamour side provides

`glamour/glamour.go:180-196`

```go
// WithTableWrap controls whether table content will wrap if too long.
func WithTableWrap(tableWrap bool) TermRendererOption

// WithTableWidth sets the width for tables.
// If set to 0, tables are rendered at their natural content width without wrapping or truncation.
func WithTableWidth(tableWidth int) TermRendererOption
```

Consumed in `glamour/ansi/table.go:60-73` (`table.New().Wrap(wrap)` +
`Width(...)`) and `:149` (`isUnconstrained := TableWidth != nil && *TableWidth == 0`),
which routes unconstrained tables through a placeholder buffer
(`\x00GLAMOUR_TABLE_n\x00`, substituted in `ansi/blockelement.go:44-46`) so
they bypass the word-wrap pass.

## Build & run

```bash
cd ~/go/glow
go build -o /tmp/glow-dev .
/tmp/glow-dev -p --table-wrap=false test_table.md
```

## Test

```bash
go test ./...                       # glow + ui packages
go test ./ui -run TestPagerSearchAndFilterIntegration -v
task test                           # same thing, via Taskfile
task lint                           # golangci-lint (config: .golangci.yml)
gofmt -l .                          # currently flags glow_test.go and ui/styles.go
```

## Behaviour harness

```bash
./scripts/local-checks.sh            # all four groups
./scripts/local-checks.sh tables     # wrap/width matrix vs base
./scripts/local-checks.sh width      # terminal width detection vs base
./scripts/local-checks.sh config     # config-vs-flag precedence
./scripts/local-checks.sh search     # byte vs cell offset proof
```

It builds the branch binary plus a one-off binary of `BASE_REF` (default
`7b2431d`, via a temporary `git worktree`), keeps everything under
`/tmp/glow-local-checks`, and never touches `~/.config/glow/glow.yml`.
Override the baseline with `BASE_REF=<sha> ./scripts/local-checks.sh`.

## Debugging width / flag plumbing

The fastest way to see what glow actually resolved is a temporary print in
`validateOptions` (do it in a copy, not the working tree):

```go
fmt.Fprintf(os.Stderr, "width=%d detect=%d COLUMNS=%q tty=%v\n",
    width, detectTerminalWidth(), os.Getenv("COLUMNS"),
    term.IsTerminal(int(os.Stdout.Fd())))
```

and for the table options:

```go
fmt.Fprintf(os.Stderr, "tableWrap=%v tableWidth=%d changed=%v isSet=%v\n",
    tableWrap, tableWidth, cmd.Flags().Changed("table-width"), viper.IsSet("tableWidth"))
```

Glow's own log: `task log` (`~/.cache/glow/glow.log` on Linux).

## Config gotchas while testing

- If no `glow.yml` exists, glow **creates** one with `width: 80` on first run,
  which disables width auto-detection for every later run. Use an explicit
  `width: 0` config when testing width detection.
- `~/.config/glow/glow.yml` on this machine currently contains
  `table_wrap: false` and `table_width: 0`, which (per
  [known-issues.md #3](known-issues.md#3-config-file-overrides-cli-flags))
  makes the CLI flags no-ops. Test with a throwaway `HOME` when you need the
  flags to take effect.
- Config lookup order: `GLOW_CONFIG_HOME` → `$XDG_CONFIG_HOME/glow` → the
  `gap` user-scope dirs. `viper.AutomaticEnv()` with prefix `GLOW` is also on.

## Before any upstream PR

1. ~~Push the glamour work to a fork~~ — done: `dpavlin/glamour` exists and
   `feat/unwrapped-tables` @ `233e531` is pushed to it.
2. Open the glamour PR (`gh pr create -R charmbracelet/glamour -h dpavlin:feat/unwrapped-tables`);
   get `WithTableWrap`/`WithTableWidth` merged or published as a tagged
   pseudo-version. This is the critical path — see
   [known-issues.md #1](known-issues.md#1--the-glamour-dependency-is-a-local-replace-branch-now-pushed-to-a-fork).
3. Remove `replace charm.land/glamour/v2 => ../glamour` from `glow/go.mod` and
   require the published version.
4. Fix [known-issues.md](known-issues.md) items #2, #3, #4 (and ideally #5–#10).
5. `gofmt -w` the touched files; `task lint`.
6. Move or delete `test_table.md`; add real fixtures under `testdata/`.
7. Update `README.md` (word-wrap / config-file sections) and the `--help` text.
