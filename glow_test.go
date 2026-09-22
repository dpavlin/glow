package main

import (
	"strings"
	"testing"

	"charm.land/glamour/v2"
	"charm.land/glow/v3/utils"
	"github.com/charmbracelet/x/ansi"
)

func TestGlowFlags(t *testing.T) {
	tt := []struct {
		args  []string
		check func() bool
	}{
		{
			args: []string{"-p"},
			check: func() bool {
				return pager
			},
		},
		{
			args: []string{"-s", "light"},
			check: func() bool {
				return style == "light"
			},
		},
		{
			args: []string{"-w", "40"},
			check: func() bool {
				return width == 40
			},
		},
		{
			args: []string{"--table-wrap=false"},
			check: func() bool {
				return !tableWrap
			},
		},
		{
			args: []string{"--table-width", "100"},
			check: func() bool {
				return tableWidth == 100
			},
		},
	}

	for _, v := range tt {
		err := rootCmd.ParseFlags(v.args)
		if err != nil {
			t.Fatal(err)
		}
		if !v.check() {
			t.Errorf("Parsing flag failed: %s", v.args)
		}
	}
}

func TestTableUnwrappedWithTextWrapped(t *testing.T) {
	md := `This is a long introductory paragraph that should wrap nicely at sixty characters terminal width while the table below extends horizontally without wrapping.

| Server Name | IP Address | Status | Description |
| --- | --- | --- | --- |
| srv-alpha-01 | 192.168.1.100 | ONLINE | Main production application server hosting primary API endpoints |
`
	options := []glamour.TermRendererOption{
		utils.GlamourStyle("dark", false),
		glamour.WithWordWrap(60),
		glamour.WithTableWrap(false),
		glamour.WithTableWidth(0),
	}
	r, err := glamour.NewTermRenderer(options...)
	if err != nil {
		t.Fatal(err)
	}
	out, err := r.Render(md)
	if err != nil {
		t.Fatal(err)
	}

	clean := ansi.Strip(out)
	// Paragraph should be wrapped (lines <= 60 chars)
	// Table should have the full description on a single line without wrapping or ellipsis
	if !strings.Contains(clean, "Main production application server hosting primary API endpoints") {
		t.Fatalf("table description was truncated or mangled:\n%s", clean)
	}
	if strings.Contains(clean, "…") {
		t.Fatalf("table was truncated with ellipsis:\n%s", clean)
	}
}


