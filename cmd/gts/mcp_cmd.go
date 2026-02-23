package main

import (
	"errors"
	"flag"
	"os"

	"gts-suite/internal/mcp"
)

func runMCP(args []string) error {
	flags := flag.NewFlagSet("gtsmcp", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)

	args = normalizeFlagArgs(args, map[string]bool{
		"-root":          true,
		"--root":         true,
		"-cache":         true,
		"--cache":        true,
		"-allow-writes":  false,
		"--allow-writes": false,
	})

	root := flags.String("root", ".", "default root path for tool calls")
	cachePath := flags.String("cache", "", "default cache path for tool calls")
	allowWrites := flags.Bool("allow-writes", false, "allow MCP tools to mutate files (e.g. gts_refactor write mode)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return errors.New("usage: gtsmcp [--root .] [--cache .gts/index.json] [--allow-writes]")
	}

	service := mcp.NewServiceWithOptions(*root, *cachePath, mcp.ServiceOptions{
		AllowWrites: *allowWrites,
	})
	return mcp.RunStdio(service, os.Stdin, os.Stdout, os.Stderr)
}
