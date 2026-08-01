//go:build !windows

package cmd

import (
	"os"
	"os/signal"
	"syscall"
)

func executionSignals() []os.Signal {
	return []os.Signal{os.Interrupt, syscall.SIGTERM}
}

func executionSignalExitCode(value os.Signal) int {
	if value == syscall.SIGTERM {
		return 143
	}
	return 130
}

func ignoreBrokenPipeSignal() func() {
	signal.Ignore(syscall.SIGPIPE)
	return func() {}
}
