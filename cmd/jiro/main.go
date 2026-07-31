package main

import (
	"os"

	command "github.com/timonwong/jiro/internal/cmd"
)

func main() {
	os.Exit(command.Execute(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
