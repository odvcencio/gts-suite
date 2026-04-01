package main

import (
	"github.com/spf13/cobra"
)

type exitCodeError struct {
	code int
	err  error
}

func (e exitCodeError) Error() string {
	if e.err == nil {
		return "command failed"
	}
	return e.err.Error()
}

func (e exitCodeError) ExitCode() int {
	if e.code <= 0 {
		return 1
	}
	return e.code
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:     "gts",
		Short:   "Structural code analysis toolkit",
		Version: version,
	}
	root.PersistentFlags().Bool("include-generated", false, "include generated files in analysis output")
	root.PersistentFlags().String("generator", "", "filter to a specific generator name (e.g. protobuf, mockgen, human)")
	root.PersistentFlags().String("federation", "", "directory containing .gtsindex files for multi-repo federated analysis")

	root.AddCommand(
		newIndexGroup(),
		newSearchGroup(),
		newGraphGroup(),
		newAnalyzeGroup(),
		newTransformGroup(),
		newMCPCmd(),
		newInitCmd(),
	)
	return root
}
