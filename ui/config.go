package ui

// Config contains TUI-specific configuration.
type Config struct {
	ShowAllFiles     bool
	ShowLineNumbers  bool
	Gopath           string `env:"GOPATH"`
	HomeDir          string `env:"HOME"`
	GlamourMaxWidth  uint
	GlamourStyle     string `env:"GLAMOUR_STYLE"`
	EnableMouse      bool
	PreserveNewLines bool
	TableWrap        bool `env:"GLOW_TABLE_WRAP" envDefault:"true"`
	TableWidth       uint `env:"GLOW_TABLE_WIDTH"`
	ChopLongLines    bool `env:"GLOW_CHOP_LONG_LINES"`

	// Working directory or file path
	Path string

	// For debugging the UI
	GlamourEnabled bool `env:"GLOW_ENABLE_GLAMOUR" envDefault:"true"`
}
