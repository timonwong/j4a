//go:build windows

package cmd

import (
	"errors"
	"syscall"
)

const windowsErrorNoData syscall.Errno = 232

func isBrokenPipe(err error) bool {
	return errors.Is(err, syscall.ERROR_BROKEN_PIPE) || errors.Is(err, windowsErrorNoData)
}
