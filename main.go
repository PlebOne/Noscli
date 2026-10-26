package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/charmbracelet/bubbletea"
	"noscli/pkg/tui"
)

const usage = `Usage: noscli [options]

A terminal Nostr client. Run without options to start the TUI.

Options:
  -h, --help       Show this help and exit
  -v, --version    Show the version and exit
      --debug      Write a debug log to $XDG_STATE_HOME/noscli/debug.log
                   (default ~/.local/state/noscli/debug.log), replaced on each start
`

type options struct {
	help    bool
	version bool
	debug   bool
}

func parseArgs(args []string) (options, error) {
	var opts options
	for _, arg := range args {
		switch arg {
		case "-h", "--help":
			opts.help = true
		case "-v", "--version":
			opts.version = true
		case "--debug":
			opts.debug = true
		default:
			return opts, fmt.Errorf("unknown argument: %s", arg)
		}
	}
	return opts, nil
}

// debugLogPath returns the debug log location, following XDG_STATE_HOME
func debugLogPath() (string, error) {
	stateDir := os.Getenv("XDG_STATE_HOME")
	if stateDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		stateDir = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(stateDir, "noscli", "debug.log"), nil
}

func main() {
	opts, err := parseArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "noscli: %v\n\n%s", err, usage)
		os.Exit(2)
	}
	if opts.help {
		fmt.Print(usage)
		return
	}
	if opts.version {
		fmt.Printf("noscli %s\n", tui.Version)
		return
	}

	// Logging would corrupt the TUI on stderr, so discard it unless --debug is set
	log.SetOutput(io.Discard)
	if opts.debug {
		logPath, err := debugLogPath()
		if err == nil {
			os.MkdirAll(filepath.Dir(logPath), 0700)
			// Truncate on start so the log doesn't grow without bound
			logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
			if err == nil {
				log.SetOutput(logFile)
				log.Printf("========== Noscli Started ==========")
				defer logFile.Close()
			}
		}
	}

	m := tui.NewModel()
	p := tea.NewProgram(m, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		fmt.Printf("Alas, there's been an error: %v", err)
		os.Exit(1)
	}
}
