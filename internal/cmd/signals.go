package cmd

import (
	"context"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
)

func executionSignalContext() (context.Context, func(), func() int) {
	ctx, cancel := context.WithCancel(context.Background())
	registered := executionSignals()
	restoreBrokenPipe := ignoreBrokenPipeSignal()
	native := make(chan os.Signal, 1)
	done := make(chan struct{})
	signal.Notify(native, registered...)
	var code atomic.Int32
	go func() {
		select {
		case value := <-native:
			code.Store(int32(executionSignalExitCode(value)))
			cancel()
		case <-done:
		}
	}()
	var once sync.Once
	stop := func() {
		once.Do(func() {
			signal.Stop(native)
			restoreBrokenPipe()
			close(done)
			cancel()
		})
	}
	return ctx, stop, func() int { return int(code.Load()) }
}

func isAPIInvocation(args []string) bool {
	valueFlags := map[string]struct{}{
		"--config": {}, "-c": {}, "--profile": {}, "--host": {}, "--username": {}, "--auth-type": {}, "--output": {}, "-o": {},
	}
	for index := 0; index < len(args); index++ {
		argument := args[index]
		if argument == "--" {
			return index+1 < len(args) && args[index+1] == "api"
		}
		if _, takesValue := valueFlags[argument]; takesValue {
			index++
			continue
		}
		if strings.HasPrefix(argument, "--config=") || strings.HasPrefix(argument, "--profile=") ||
			strings.HasPrefix(argument, "--host=") || strings.HasPrefix(argument, "--username=") ||
			strings.HasPrefix(argument, "--auth-type=") || strings.HasPrefix(argument, "--output=") ||
			(strings.HasPrefix(argument, "-c") && argument != "-c") || (strings.HasPrefix(argument, "-o") && argument != "-o") {
			continue
		}
		if strings.HasPrefix(argument, "-") {
			continue
		}
		return argument == "api"
	}
	return false
}

func hasExplicitOutputFlag(args []string) bool {
	valueFlags := map[string]struct{}{
		"--config": {}, "-c": {}, "--profile": {}, "--host": {}, "--username": {}, "--auth-type": {},
		"--method": {}, "-X": {}, "--input": {}, "--string-field": {}, "-f": {}, "--field": {}, "-F": {},
		"--form": {}, "--header": {}, "-H": {}, "--timeout": {},
	}
	for index := 0; index < len(args); index++ {
		argument := args[index]
		if argument == "--" {
			return false
		}
		if argument == "--output" || argument == "-o" || strings.HasPrefix(argument, "--output=") ||
			(strings.HasPrefix(argument, "-o") && argument != "-o") {
			return true
		}
		if _, takesValue := valueFlags[argument]; takesValue {
			index++
			continue
		}
	}
	return false
}
