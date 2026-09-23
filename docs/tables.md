# Unwrapped tables

Everything the branch exposes for controlling how tables are laid out, and what
each knob actually does once it reaches glamour.

## Knobs

### CLI flags (`main.go`)

```
--table-wrap    bool   default true   wrap table cells to the render width (use --table-wrap=false for natural content width)
--table-width  uint    default 0     lay tables out at this width; 0 means natural content width
```

Bound to viper as `tableWrap` / `tableWidth`, with defaults `tableWrap=true`,
`tableWidth=0`.

### Config file keys

Two spellings are honoured, both **below** the CLI flags
(`resolveTableSettings` in `main.go`):

| spelling | notes |
|---|---|
| `tableWrap` / `tableWidth` | via the pflag binding |
| `table_wrap` / `table_width` | extra config-file spelling, applied only when the flag was not passed |

Example (`~/.config/glow/glow.yml`):

```yaml
style: "auto"
width: 0
table_wrap: false   # keep tables unwrapped
table_width: 0      # 0 = natural content width
```

`glow --table-wrap=true` overrides the config, as it always should have.

### Env vars

```
GLOW_TABLE_WRAP    bool   (declared in ui/config.go, wired through viper.BindEnv)
GLOW_TABLE_WIDTH   uint
```

Precedence: **flag > env > config > default**. Verified in
`./scripts/local-checks.sh config`, section 3b.

## What glamour does with the options

Glamour's table element (`../glamour/ansi/table.go:60-73`, `:149`) interprets
the two pointers like this:

| `WithTableWrap` | `WithTableWidth` | glamour behaviour |
|---|---|---|
| unset | unset | wrap cells to the block width (upstream default) |
| `true` | unset | wrap cells to the block width |
| `false` | unset | **truncate** each cell with `…` |
| any | `0` | **unconstrained** — natural content width, no wrap, no ellipsis |
| any | `N > 0` | lay the table out at exactly `N` columns |

Note the asymmetry: `TableWrap=false` alone does *not* mean "unwrapped", it
means "truncated". Unconstrained output requires `TableWidth == 0` (non-nil).
In the unconstrained path glamour renders the table into a side buffer and
leaves a `\x00GLAMOUR_TABLE_n\x00` placeholder that `ansi/blockelement.go`
substitutes back in, so the table escapes the word-wrap pass entirely.

## What glow sends

Both the CLI and the TUI go through one helper, `utils.TableOptions`
(`utils/table.go`), so they cannot drift apart:

```go
func TableOptions(wrap bool, width uint) []glamour.TermRendererOption {
	switch {
	case width > 0:
		return []glamour.TermRendererOption{
			glamour.WithTableWrap(wrap),
			glamour.WithTableWidth(int(width)),
		}
	case !wrap:
		return []glamour.TermRendererOption{
			glamour.WithTableWrap(false),
			glamour.WithTableWidth(0),
		}
	default:
		// Upstream behaviour: let glamour wrap cells to the render width.
		return nil
	}
}
```

Measured (`--width 40`, two very wide cells — `./scripts/local-checks.sh tables`):

| invocation | max cells | lines | meaning |
|---|---|---|---|
| default | 38 | 11 | wrapped to the render width — same as base `7b2431d` |
| `--table-wrap=true` | 38 | 11 | wrapped |
| `--table-wrap=false` | 116 | 6 | natural content width |
| `--table-width 60` | 62 | 10 | laid out at 60 |
| `--table-wrap=true --table-width 60` | 62 | 10 | laid out at 60, wrapping |
| `--table-wrap=false --table-width 0` | 116 | 6 | natural content width |

Covered by `utils.TestTableOptionsDefaultKeepsUpstreamWrapping`,
`TestTableOptionsNaturalWidth`, `TestTableOptionsExplicitWidth`.

## Recipes

| want | do |
|---|---|
| upstream behaviour (cells wrap to the render width) | default, or `--table-wrap=true` |
| unwrapped tables, scroll horizontally | `--table-wrap=false` |
| wrap tables to a fixed width | `--table-width 60` |
| truncate wide cells with `…` | `--table-wrap=false --table-width 60` |
| make it permanent | `table_wrap: false` in `glow.yml`, or `GLOW_TABLE_WRAP=false` |

## History (what the fix replaced)

Before the fix the CLI used:

```go
} else if cmd.Flags().Changed("table-width") || viper.IsSet("tableWidth") {
    options = append(options, glamour.WithTableWidth(int(tableWidth)))
}
```

`viper.IsSet("tableWidth")` is true even when the user never set it (the key is
bound to a pflag), so `WithTableWidth(0)` was always sent and every table
rendered unconstrained — including `--table-wrap=true`. Meanwhile the TUI used
`else if cfg.TableWidth > 0` and rendered wrapped, so the same config produced
two different results. See
[known-issues.md](known-issues.md) items #2 and #6.
