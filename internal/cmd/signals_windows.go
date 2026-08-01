//go:build windows

package cmd

import "os"

func executionSignals() []os.Signal {
	return []os.Signal{os.Interrupt}
}

func executionSignalExitCode(os.Signal) int {
	return 130
}

func ignoreBrokenPipeSignal() func() {
	return func() {}
}
