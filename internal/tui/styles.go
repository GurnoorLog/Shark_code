package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	Teal    = lipgloss.Color("#00e5c8")
	Cyan    = lipgloss.Color("#5ad1f8")
	White   = lipgloss.Color("#e8f6ff")
	Bubble  = lipgloss.Color("#a9d9ef")
	Coral   = lipgloss.Color("#ff7b72")
	AzureBG = lipgloss.Color("#0b5d9e")
	DeepBG  = lipgloss.Color("#0a2f52")
	CodeBG  = lipgloss.Color("#0a1f33")
	CodeFg  = lipgloss.Color("#9fd6ff")
	Mono    = lipgloss.Color("#c8e6ff")
)

var waterColors = []lipgloss.Color{
	lipgloss.Color("#012a4a"),
	lipgloss.Color("#013a63"),
	lipgloss.Color("#01497c"),
	lipgloss.Color("#014f86"),
	lipgloss.Color("#2a6f97"),
	lipgloss.Color("#2c7da0"),
	lipgloss.Color("#468faf"),
	lipgloss.Color("#61a5c2"),
}

var (
	HeaderStyle = lipgloss.NewStyle().
			Background(AzureBG).
			Foreground(White).
			Bold(true)

	TitleStyle = lipgloss.NewStyle().
			Foreground(Cyan).
			Bold(true)

	TaglineStyle = lipgloss.NewStyle().
			Foreground(Bubble)

	UserLabel = lipgloss.NewStyle().
			Foreground(Cyan).
			Bold(true)

	SharkLabel = lipgloss.NewStyle().
			Foreground(Teal).
			Bold(true)

	// UserChip / SharkChip render the "you" / "shark" role chips that head
	// each message, opencode-style.
	UserChip = lipgloss.NewStyle().
			Foreground(White).
			Background(Cyan).
			Bold(true).
			Padding(0, 1)

	SharkChip = lipgloss.NewStyle().
			Foreground(DeepBG).
			Background(Teal).
			Bold(true).
			Padding(0, 1)

	BodyStyle = lipgloss.NewStyle().
			Foreground(White)

	HintStyle = lipgloss.NewStyle().
			Foreground(Bubble).
			Italic(true)

	ErrorStyle = lipgloss.NewStyle().
			Foreground(Coral).
			Bold(true)

	CoralLabel = lipgloss.NewStyle().
			Foreground(Coral).
			Bold(true)

	PromptStyle = lipgloss.NewStyle().
			Foreground(Teal).
			Bold(true)

	StatusBar = lipgloss.NewStyle().
			Background(DeepBG).
			Foreground(Bubble).
			Bold(true)

	QueueChip = lipgloss.NewStyle().
			Foreground(DeepBG).
			Background(Cyan).
			Bold(true).
			Padding(0, 1)

	MenuSelStyle = lipgloss.NewStyle().
			Foreground(Teal).
			Bold(true).
			Background(AzureBG).
			Padding(0, 1)

	// CodeBlockStyle wraps fenced code blocks in a dark panel with mono text.
	CodeBlockStyle = lipgloss.NewStyle().
			Background(CodeBG).
			Foreground(CodeFg).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#1a4a7a")).
			Padding(0, 2)

	InlineCodeStyle = lipgloss.NewStyle().
			Foreground(CodeFg).
			Background(CodeBG).
			Italic(false)

	ListBullet = lipgloss.NewStyle().Foreground(Coral).Bold(true).Render("▸ ")

	// LangChip sits above a code panel and names the language.
	LangChip = lipgloss.NewStyle().
			Foreground(DeepBG).
			Background(Cyan).
			Bold(true).
			Padding(0, 1)

	// QuoteBar styles markdown blockquotes as a teal bar + pale body.
	QuoteBar = lipgloss.NewStyle().
			Foreground(Teal).
			Bold(true).
			Render("▐ ")

	// WaveDivider is a faint wave line between chat messages.
	WaveDivider = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#2c7da0")).
			Render("「 ~ ~ ~ § ~ ~ § ~ ~ 」")
)

// Panel styles for the opencode-style right info panel.
var (
	PanelBG  = lipgloss.Color("#0b2b4d")
	PanelRule = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#1a5f7a")).
			Render("─")

	PanelLabel = lipgloss.NewStyle().
			Foreground(Bubble).
			Bold(true).
			Background(lipgloss.Color("#13354f"))

	PanelValue = lipgloss.NewStyle().
			Foreground(White)

	PanelValueDim = lipgloss.NewStyle().
			Foreground(Bubble)

	PanelBar = lipgloss.NewStyle().
			Foreground(Teal).
			Background(lipgloss.Color("#0e4a57"))
)

// SharkMark is the compact one-line shark used on terminals too short for
// the full braille art — keeps the theme without crowding the controls.
func SharkMark() string {
	return lipgloss.NewStyle().Foreground(Teal).Bold(true).Render(">≈((((º>")
}

// SharkLogo renders a detailed ASCII great white for the welcome screen.
func SharkLogo() string {
	rows := []string{
		`⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠰⣶⣶⣿⣷⣶⣶⣶⣦⣤⣀`,
		`⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠈⢻⣿⣿⣿⣿⣿⣿⣿⣿⣿⣶⣄⡀`,
		`⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠙⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣶⣄`,
		`⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠘⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣷⣶⣶⣶⣶⣶⣶⣶⣶⣦⣤⣄⣀`,
		`⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⢹⣿⣿⣿⣿⣿⣿⠿⠟⠛⠛⠉⠉⣉⣉⣉⣉⣉⣛⣛⡻⠿⣿⣿⣿⣿⣷⣶⣤⣀`,
		`⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⣨⣿⣿⠟⠋⢁⣀⣤⣶⣾⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣶⣄⡀`,
		`⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⣠⣾⡿⠋⢀⣤⣾⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣦⣄⡀`,
		`⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⣴⡿⠋⣀⣴⣿⣿⣿⢿⣿⣿⣿⣿⣿⣿⣿⣿⣿⡟⠿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣷⣦⣄`,
		`⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⢠⣾⠏⢀⣴⣿⣿⣿⣿⠏⣼⣿⣿⣿⣿⣿⢻⣿⣿⣿⣿⣤⣈⣙⣻⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⡿⠿⠛⠛⣻⣿⣿⠆`,
		`⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⢠⡿⠃⢠⣿⣿⣿⣿⣿⡏⠀⢻⣿⣿⠘⣿⣿⠸⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣯⣥⣤⣤⣴⣾⣿⠟⠁`,
		`⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠾⠃⢀⣿⣿⣿⣿⣿⣿⣧⡀⠈⠻⣿⣦⠈⠻⠄⢹⣿⣿⣿⡿⠟⠋⠉⠉⠉⠉⠉⠉⠉⠉⠉⠉⠉⠉⠉⠉⠉⠉⠉⣽⣿⠟⠁`,
		`⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⢀⣨⣿⣿⣿⣿⣿⣿⣿⣿⣶⣤⠀⠀⠀⢀⣴⠾⠛⠉⠁⠀⣄⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⣠⣾⠟⠁`,
		`⠀⠀⠀⠀⠀⠀⠀⠀⢀⣠⣴⣾⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⠟⠁⠀⠀⠀⠉⠀⠀⠀⠀⠀⢸⣿⣦⣶⣰⣧⣠⣦⠀⣴⣆⢀⣦⣀⣴⣄⣴⣿⣿⣿⣿⣶⣶⣤⣄⡀`,
		`⠀⠀⠀⠀⣀⣠⣴⣾⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⡿⠟⠋⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠸⣿⡿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⠿⠿⠿⠿⠟⠛⠛⠛⠛⠛⠛⠛⠓`,
		`⣀⣤⣶⣿⣿⣿⣿⣿⣿⣿⠿⠿⠿⠟⠛⠋⠉⠀⠀⣀⣠⣤⣶⡶⠶⠶⠶⠶⠶⢶⣶⣦⣤⣄⣙⠀⠘⠟⠙⢿⡿⢻⣿⡿⣿⡿⢿⡇`,
		`⠀⠉⠉⠁⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⣠⣾⣿⠟⠉⠀⠀⠀⠀⠀⠀⠀⠀⠈⠉⠙⠛⠻⢿⣷⣶⣤⣀⡀⠀⠉⠀⠈⣁⣾⡇`,
		`⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⢰⣿⡟`,
		`⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⣼⣿`,
		`⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⢸⣿⣦⣀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⢀⣠⣴⣶⡶`,
		`⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⢠⣶⣤⡘⣿⡿⠛⠀⠀⠀⠀⠀⠀⠀⣠⣴⣾⣿⣿⠟`,
		`⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⣾⣿⠛⢧⠈⠁⠀⠀⠀⠀⣀⣤⣶⣿⣿⣿⣿⡿⠁`,
		`⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠸⠿⠁⠀⠈⠀⡀⠈⠻⣿⣿⣿⣿⣿⣿⣿⡿⠋`,
		`⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⢷⡀⠀⣈⡙⠻⠿⢷⣤⡀`,
		`⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠘⣧⠀⠘⠻⢷⣶⣦⣤⡀`,
		`⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⣿⡆⠀⠀⢨⣿⣿⣿⠁`,
		`⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⢸⣷⠀⠀⢸⣿⣿⡟`,
		`⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⢿⡆⠀⢸⣿⣿⠇`,
		`⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠘⣿⠀⢸⣿⣿`,
		`⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⢹⣇⣸⣿⣿`,
		`⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⣿⣿⣿⣏`,
		`⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠸⣿⣿⣿`,
		`⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⢻⣿⣿`,
		`⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠈⣿⣿`,
		`⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⢸⣿`,
		`⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⢻⠇`,
	}
	// Body colors sweep from bright backlit blue (top) down to deep navy
	// (belly) like light hitting a real shark.
	colors := []lipgloss.Color{
		lipgloss.Color("#9bedff"), lipgloss.Color("#7fe0ff"),
		lipgloss.Color("#58d0ff"), lipgloss.Color("#2fb8f0"),
		lipgloss.Color("#0aa3e8"), lipgloss.Color("#0a8ed4"),
		lipgloss.Color("#0a78c0"), lipgloss.Color("#0a63ac"),
		lipgloss.Color("#0a4f98"), lipgloss.Color("#0a3c84"),
		lipgloss.Color("#0a2f70"), lipgloss.Color("#0a2560"),
	}
	var b strings.Builder
	for i, r := range rows {
		b.WriteString(lipgloss.NewStyle().Foreground(colors[i%len(colors)]).Bold(true).Render(r))
		if i < len(rows)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}
