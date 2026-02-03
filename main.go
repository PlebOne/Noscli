package main

import (
	"fmt"
	"os"

	"github.com/charmbracelet/bubbletea"
	"noscli/pkg/tui"
)

func main() {
	m := tui.NewModel()
	p := tea.NewProgram(m, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		fmt.Printf("Alas, there's been an error: %v", err)
		os.Exit(1)
	}
}
