package main

import (
	"fmt"
	"os"
)

func main() {
	if err := newCLI().Run(os.Args[1:]); err != nil {
		exitCode := 1
		if withCode, ok := err.(interface{ ExitCode() int }); ok {
			exitCode = withCode.ExitCode()
		}
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(exitCode)
	}
}
