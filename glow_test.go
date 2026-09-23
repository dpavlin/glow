package main

import (
	"strings"
	"testing"

	"charm.land/glamour/v2"
	"charm.land/glow/v3/utils"
	"github.com/charmbracelet/x/ansi"
	"github.com/spf13/viper"
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

// A config file must never override a flag passed on the command line.
func TestResolveTableSettingsPrecedence(t *testing.T) {
	// Put the snake_case keys in viper's *config* layer, which is where a real
	// glow.yml ends up. viper.Set would be an override, and overrides outrank
	// flags, so it would not model the real situation.
	viper.SetConfigType("yaml")
	if err := viper.MergeConfig(strings.NewReader("table_wrap: false\ntable_width: 0\n")); err != nil {
		t.Fatalf("merge config: %v", err)
	}
	t.Cleanup(func() {
		viper.Set("table_wrap", nil)
		viper.Set("table_width", nil)
	})

	// Flags explicitly given: they win over the config file.
	if err := rootCmd.ParseFlags([]string{"--table-wrap=true", "--table-width", "80"}); err != nil {
		t.Fatal(err)
	}
	wrap, width := resolveTableSettings(rootCmd.Flags().Changed)
	if !wrap {
		t.Errorf("--table-wrap=true should beat table_wrap: false, got wrap=%v", wrap)
	}
	if width != 80 {
		t.Errorf("--table-width 80 should beat table_width: 0, got width=%d", width)
	}

	// No flags given: the config file still applies.
	neverChanged := func(string) bool { return false }
	wrap, width = resolveTableSettings(neverChanged)
	if wrap {
		t.Errorf("table_wrap: false should apply when no flag is given, got wrap=%v", wrap)
	}
	if width != 0 {
		t.Errorf("table_width: 0 should apply when no flag is given, got width=%d", width)
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
