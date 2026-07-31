//go:build !windows

package cmd

import "os"

func openControllingOutput() (*os.File, error) {
	return os.Open("/dev/tty")
}
