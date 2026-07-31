package cmd

import (
	"io"
	"os"
	"strings"

	"github.com/timonwong/jiro/internal/apperr"
	"github.com/timonwong/jiro/internal/output"
	"golang.org/x/term"
)

const fallbackTerminalWidth = 80

type outputTerminalDetector interface {
	Detect(io.Writer, bool) output.Terminal
}

type streamOutputTerminal struct{}

func (streamOutputTerminal) Detect(writer io.Writer, force bool) output.Terminal {
	if file, ok := writer.(*os.File); ok && term.IsTerminal(int(file.Fd())) {
		return output.Terminal{IsTTY: true, Width: terminalWidth(file)}
	}
	if !force {
		return output.Terminal{}
	}
	terminal := output.Terminal{IsTTY: true, Width: fallbackTerminalWidth}
	file, err := openControllingOutput()
	if err != nil {
		return terminal
	}
	defer file.Close()
	terminal.Width = terminalWidth(file)
	return terminal
}

func terminalWidth(file *os.File) int {
	width, _, err := term.GetSize(int(file.Fd()))
	if err != nil || width <= 0 {
		return fallbackTerminalWidth
	}
	return width
}

func parseForceTTY() (bool, error) {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("JIRO_FORCE_TTY"))) {
	case "", "0", "false", "no", "off":
		return false, nil
	case "1", "true", "yes", "on":
		return true, nil
	default:
		return false, apperr.New(apperr.KindConfig, "JIRO_FORCE_TTY must be a boolean")
	}
}
