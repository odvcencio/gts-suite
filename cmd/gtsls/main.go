package main

import (
	"fmt"
	"os"

	"github.com/odvcencio/gts-suite/pkg/lsp"
)

var version = "0.1.0"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Println("gtsls " + version)
		os.Exit(0)
	}

	svc := lsp.NewService()
	srv := lsp.NewServer(os.Stdin, os.Stdout, os.Stderr)
	svc.Register(srv)

	if err := srv.Serve(); err != nil {
		fmt.Fprintf(os.Stderr, "gtsls: %v\n", err)
		os.Exit(1)
	}
}
