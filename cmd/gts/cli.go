package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/odvcencio/fluffyui/keybind"
)

type commandSpec struct {
	ID      string
	Aliases []string
	Summary string
	Usage   string
	Run     func(args []string) error
}

type cli struct {
	registry   *keybind.CommandRegistry
	specs      map[string]commandSpec
	aliasToID  map[string]string
	invokeArgs []string
	invokeErr  error
}

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

func newCLI() *cli {
	c := &cli{
		registry:  keybind.NewRegistry(),
		specs:     make(map[string]commandSpec),
		aliasToID: make(map[string]string),
	}

	commands := []commandSpec{
		{
			ID:      "gtsindex",
			Aliases: []string{"index"},
			Summary: "Build a structural index and optionally cache it",
			Usage:   "gtsindex [path] [--out .gts/index.json] [--incremental] [--watch] [--subfile-incremental] [--poll] [--interval 2s] [--report-changes] [--once-if-changed] [--json]",
			Run:     runIndex,
		},
		{
			ID:      "gtsmap",
			Aliases: []string{"map"},
			Summary: "Print structural summaries for indexed files",
			Usage:   "gtsmap [path] [--cache .gts/index.json] [--json]",
			Run:     runMap,
		},
		{
			ID:      "gtsfiles",
			Aliases: []string{"files"},
			Summary: "List/index files with structural density filters",
			Usage:   "gtsfiles [path] [--cache .gts/index.json] [--language go] [--min-symbols 0] [--sort symbols|imports|size|path] [--top 50] [--json]",
			Run:     runFiles,
		},
		{
			ID:      "gtsstats",
			Aliases: []string{"stats"},
			Summary: "Report structural codebase metrics from an index",
			Usage:   "gtsstats [path] [--cache .gts/index.json] [--top 10] [--json]",
			Run:     runStats,
		},
		{
			ID:      "gtsdeps",
			Aliases: []string{"deps"},
			Summary: "Analyze dependency graph from structural imports",
			Usage:   "gtsdeps [path] [--cache .gts/index.json] [--by package|file] [--top 10] [--focus node] [--depth N] [--reverse] [--edges] [--json]",
			Run:     runDeps,
		},
		{
			ID:      "gtsbridge",
			Aliases: []string{"bridge"},
			Summary: "Map cross-component dependency bridges",
			Usage:   "gtsbridge [path] [--cache .gts/index.json] [--top 20] [--focus component] [--depth N] [--reverse] [--json]",
			Run:     runBridge,
		},
		{
			ID:      "gtsgrep",
			Aliases: []string{"grep"},
			Summary: "Structural grep over indexed symbols",
			Usage:   "gtsgrep <selector> [path] [--cache .gts/index.json] [--count] [--json]",
			Run:     runGrep,
		},
		{
			ID:      "gtsrefs",
			Aliases: []string{"refs"},
			Summary: "Find indexed references by symbol name",
			Usage:   "gtsrefs <name|regex> [path] [--cache .gts/index.json] [--regex] [--count] [--json]",
			Run:     runRefs,
		},
		{
			ID:      "gtscallgraph",
			Aliases: []string{"callgraph"},
			Summary: "Build call graph edges rooted at matching callable definitions",
			Usage:   "gtscallgraph <name|regex> [path] [--cache .gts/index.json] [--regex] [--depth N] [--reverse] [--count] [--json]",
			Run:     runCallgraph,
		},
		{
			ID:      "gtsdead",
			Aliases: []string{"dead"},
			Summary: "List callable definitions with zero incoming call references",
			Usage:   "gtsdead [path] [--cache .gts/index.json] [--kind callable|function|method] [--include-entrypoints] [--include-tests] [--count] [--json]",
			Run:     runDead,
		},
		{
			ID:      "gtsquery",
			Aliases: []string{"query"},
			Summary: "Run raw tree-sitter S-expression queries across files",
			Usage:   "gtsquery <pattern> [path] [--cache .gts/index.json] [--capture name] [--count] [--json]",
			Run:     runQuery,
		},
		{
			ID:      "gtsmcp",
			Aliases: []string{"mcp"},
			Summary: "Run MCP stdio server for AI-agent tool integration",
			Usage:   "gtsmcp [--root .] [--cache .gts/index.json] [--allow-writes]",
			Run:     runMCP,
		},
		{
			ID:      "gtsdiff",
			Aliases: []string{"diff"},
			Summary: "Structural diff between two snapshots",
			Usage:   "gtsdiff [before-path] [after-path] [--before-cache file] [--after-cache file] [--json]",
			Run:     runDiff,
		},
		{
			ID:      "gtsrefactor",
			Aliases: []string{"refactor"},
			Summary: "Apply structural declaration renames (dry-run by default)",
			Usage:   "gtsrefactor <selector> <new-name> [path] [--cache file] [--engine go|treesitter] [--callsites] [--cross-package] [--write] [--json]",
			Run:     runRefactor,
		},
		{
			ID:      "gtschunk",
			Aliases: []string{"chunk"},
			Summary: "Split code into AST-boundary chunks for RAG/indexing",
			Usage:   "gtschunk [path] [--cache file] [--tokens N] [--json]",
			Run:     runChunk,
		},
		{
			ID:      "gtsscope",
			Aliases: []string{"scope"},
			Summary: "Resolve symbols in scope for a file and line",
			Usage:   "gtsscope <file> [--line N] [--root .] [--cache file] [--json]",
			Run:     runScope,
		},
		{
			ID:      "gtscontext",
			Aliases: []string{"context"},
			Summary: "Pack focused code context for a file and line",
			Usage:   "gtscontext <file> [--line N] [--tokens N] [--semantic] [--semantic-depth N] [--root .] [--cache file] [--json]",
			Run:     runContext,
		},
		{
			ID:      "gtslint",
			Aliases: []string{"lint"},
			Summary: "Run structural lint rules against indexed symbols",
			Usage:   "gtslint [path] [--cache .gts/index.json] [--rule ...] [--pattern rule.scm ...] [--fail-on-violations] [--json]",
			Run:     runLint,
		},
	}

	for _, spec := range commands {
		specCopy := spec
		c.specs[specCopy.ID] = specCopy
		c.aliasToID[specCopy.ID] = specCopy.ID
		for _, alias := range specCopy.Aliases {
			c.aliasToID[strings.ToLower(alias)] = specCopy.ID
		}

		commandID := specCopy.ID
		c.registry.Register(keybind.Command{
			ID:          commandID,
			Title:       specCopy.ID,
			Description: specCopy.Summary,
			Handler: func(ctx keybind.Context) {
				c.invokeErr = c.specs[commandID].Run(c.invokeArgs)
			},
		})
	}

	return c
}

func (c *cli) Run(args []string) error {
	if len(args) == 0 {
		c.printHelp()
		return nil
	}

	name := strings.ToLower(strings.TrimSpace(args[0]))
	if name == "-h" || name == "--help" {
		c.printHelp()
		return nil
	}
	if name == "help" {
		if len(args) == 1 {
			c.printHelp()
			return nil
		}
		id, ok := c.aliasToID[strings.ToLower(strings.TrimSpace(args[1]))]
		if !ok {
			return fmt.Errorf("unknown command %q", args[1])
		}
		c.printCommandHelp(id)
		return nil
	}

	commandID, ok := c.aliasToID[name]
	if !ok {
		return fmt.Errorf("unknown command %q", args[0])
	}
	if len(args) > 1 {
		firstArg := strings.TrimSpace(args[1])
		if firstArg == "-h" || firstArg == "--help" {
			c.printCommandHelp(commandID)
			return nil
		}
	}

	c.invokeArgs = args[1:]
	c.invokeErr = nil

	if ok := c.registry.Execute(commandID, keybind.Context{}); !ok {
		return fmt.Errorf("command %q is not executable", commandID)
	}
	return c.invokeErr
}

func (c *cli) printHelp() {
	ids := make([]string, 0, len(c.specs))
	for id := range c.specs {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	fmt.Fprintf(os.Stderr, "gts v%s\n\n", version)
	fmt.Println("gts-suite CLI")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  gts <command> [options]")
	fmt.Println()
	fmt.Println("Commands:")
	for _, id := range ids {
		spec := c.specs[id]
		fmt.Printf("  %-10s %s\n", spec.ID, spec.Summary)
	}
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  gts gtsindex . --out .gts/index.json")
	fmt.Println("  gts gtsindex . --out .gts/index.json --once-if-changed")
	fmt.Println("  gts gtsindex . --watch --subfile-incremental --interval 2s")
	fmt.Println("  gts gtsmap . --json")
	fmt.Println("  gts gtsfiles . --sort symbols --top 20")
	fmt.Println("  gts gtsstats . --top 15")
	fmt.Println("  gts gtsdeps . --by package --focus internal/query --depth 2 --reverse")
	fmt.Println("  gts gtsbridge . --focus internal/query --depth 2 --reverse")
	fmt.Println("  gts gtsgrep 'function_definition[name=/^Test/]' .")
	fmt.Println("  gts gtsrefs OldName .")
	fmt.Println("  gts gtscallgraph main . --depth 2")
	fmt.Println("  gts gtsdead . --kind callable")
	fmt.Println("  gts gtsquery '(function_declaration (identifier) @name)' .")
	fmt.Println("  gts gtsmcp --root .")
	fmt.Println("  gts gtsmcp --root . --allow-writes")
	fmt.Println("  gts gtsdiff --before-cache before.json --after-cache after.json")
	fmt.Println("  gts gtsrefactor 'function_definition[name=/^OldName$/]' NewName . --engine go --callsites --cross-package --write")
	fmt.Println("  gts gtschunk . --tokens 500 --json")
	fmt.Println("  gts gtsscope cmd/gts/main.go --line 300")
	fmt.Println("  gts gtscontext cmd/gts/main.go --line 120 --tokens 600 --semantic --semantic-depth 2")
	fmt.Println("  gts gtslint . --rule 'no function longer than 50 lines'")
	fmt.Println("  gts gtslint . --pattern ./rules/no-empty-func.scm")
	fmt.Println("  gts help gtsgrep")
}

func (c *cli) printCommandHelp(id string) {
	spec, ok := c.specs[id]
	if !ok {
		return
	}

	fmt.Printf("%s\n", spec.ID)
	fmt.Println()
	fmt.Printf("Summary: %s\n", spec.Summary)
	fmt.Printf("Usage:   gts %s\n", spec.Usage)
	if len(spec.Aliases) > 0 {
		fmt.Printf("Aliases: %s\n", strings.Join(spec.Aliases, ", "))
	}
}
