package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"shark-agent/internal/agent"
	"shark-agent/internal/config"
	"shark-agent/internal/provider"
	"shark-agent/internal/tui"
)

func main() {
	cfg := config.Load()
	reg := provider.NewRegistry(cfg)
	ag := agent.New(reg)

	fmt.Println(tui.SharkLogo())
	fmt.Println()

	m := tui.New(cfg, reg, ag)
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "sharkcode:", err)
		os.Exit(1)
	}
}
