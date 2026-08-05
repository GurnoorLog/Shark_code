package tui

import "github.com/charmbracelet/lipgloss"

var (
	Teal    = lipgloss.Color("#0ff")
	Cyan    = lipgloss.Color("#00ffff")
	DeepSea = lipgloss.Color("#002b36")
	SeaBlue = lipgloss.Color("#005f87")
	White   = lipgloss.Color("#ffffff")
	DimSea  = lipgloss.Color("#4fd6be")
	Coral   = lipgloss.Color("#ff6b6b")
	Bubble  = lipgloss.Color("#7ff5ff")
)

var waterColors = []lipgloss.Color{
	lipgloss.Color("#0a3d62"),
	lipgloss.Color("#0a4a6e"),
	lipgloss.Color("#0b587a"),
	lipgloss.Color("#0c6686"),
	lipgloss.Color("#0d7492"),
	lipgloss.Color("#0e829e"),
}

var (
	HeaderStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("#0e829e")).
			Foreground(Teal).
			Bold(true).
			Padding(0, 2).
			MaxWidth(200)

	StatusStyle = lipgloss.NewStyle().
			Foreground(Cyan).
			Bold(true)

	UserStyle = lipgloss.NewStyle().
			Foreground(White).
			Bold(true).
			Background(lipgloss.Color("#0d7492")).
			Border(lipgloss.RoundedBorder()).
			BorderBackground(lipgloss.Color("#0d7492")).
			BorderForeground(Teal).
			Padding(0, 1)

	AssistantStyle = lipgloss.NewStyle().
			Foreground(White).
			Background(lipgloss.Color("#0b587a")).
			Border(lipgloss.RoundedBorder()).
			BorderBackground(lipgloss.Color("#0b587a")).
			BorderForeground(SeaBlue).
			Padding(0, 1)

	ToolStyle = lipgloss.NewStyle().
			Foreground(DimSea).
			Background(lipgloss.Color("#0a4a6e")).
			Border(lipgloss.RoundedBorder()).
			BorderBackground(lipgloss.Color("#0a4a6e")).
			BorderForeground(Coral).
			Padding(0, 1)

	HintStyle = lipgloss.NewStyle().
			Foreground(Bubble).
			Italic(true)

	ErrorStyle = lipgloss.NewStyle().
			Foreground(Coral).
			Bold(true)

	InputStyle = lipgloss.NewStyle().
			Foreground(White).
			Background(lipgloss.Color("#0a3d62")).
			Padding(0, 1)

	PromptStyle = lipgloss.NewStyle().
			Foreground(Teal).
			Bold(true)
)

func SharkLogo() string {
	return lipgloss.NewStyle().
		Foreground(Teal).
		Bold(true).
		Render(`      __________________ 
   .-'                  '-.
  /          __          \
 |    o      |     o      |
  \        __|__          /
   '-.__            __.-'
        '=========='`)
}
