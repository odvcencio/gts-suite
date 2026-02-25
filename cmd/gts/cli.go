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
	root.AddCommand(
		newIndexCmd(),
		newMapCmd(),
		newFilesCmd(),
		newStatsCmd(),
		newDepsCmd(),
		newBridgeCmd(),
		newGrepCmd(),
		newRefsCmd(),
		newCallgraphCmd(),
		newDeadCmd(),
		newQueryCmd(),
		newMCPCmd(),
		newDiffCmd(),
		newRefactorCmd(),
		newChunkCmd(),
		newScopeCmd(),
		newContextCmd(),
		newLintCmd(),
		newCapaCmd(),
		newSimilarityCmd(),
		newYaraCmd(),
	)
	return root
}
