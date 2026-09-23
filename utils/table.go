package utils

import "charm.land/glamour/v2"

// TableOptions maps glow's table wrap/width settings onto glamour renderer
// options. It is shared by the CLI and the TUI so both render tables the same
// way from the same configuration.
//
// glamour's two options are not symmetric, which is why this mapping exists:
//
//	width > 0            lay the table out at exactly `width` columns. `wrap`
//	                     decides what happens to cells that overflow: wrapped
//	                     onto more lines (true) or truncated with an ellipsis
//	                     (false).
//	width == 0 && !wrap  render at natural content width: no wrapping and no
//	                     truncation. The table bypasses the word-wrap pass.
//	width == 0 && wrap   upstream default: wrap cells to the current render
//	                     width.
//
// Note that WithTableWrap(false) on its own means "truncate", not "unwrapped":
// getting natural content width requires WithTableWidth(0).
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
