package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/charmbracelet/bubbletea"
	"noscli/pkg/tui"
)

func main() {
	// Set up debug logging to file
	home, err := os.UserHomeDir()
	if err == nil {
		logPath := filepath.Join(home, ".config", "noscli", "debug.log")
		os.MkdirAll(filepath.Dir(logPath), 0700)
		logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if err == nil {
			log.SetOutput(logFile)
			log.Printf("\n\n========== Noscli Started ==========")
			defer logFile.Close()
		}
	}

	m := tui.NewModel()
	p := tea.NewProgram(m, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		fmt.Printf("Alas, there's been an error: %v", err)
		os.Exit(1)
	}
}
