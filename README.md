# gts-suite

Bootstrap implementation of a gotreesitter-style CLI suite with shared indexing primitives.

## Implemented commands

- `gtsindex`: Build and cache a structural index.
- `gtsmap`: Emit structural summaries for parsed files.
- `gtsfiles`: List files with structural density filters/sorting.
- `gtsstats`: Report structural codebase metrics from an index.
- `gtsdeps`: Analyze import dependency graph (package or file level).
- `gtsbridge`: Map cross-component dependency bridges.
- `gtsgrep`: Query symbols using structural selectors.
- `gtsdiff`: Compare structural changes between two snapshots.
- `gtsrefactor`: Apply structural declaration renames (dry-run by default).
- `gtschunk`: Split code into AST-boundary chunks for retrieval/indexing.
- `gtsscope`: Resolve symbols in scope at a file + line.
- `gtscontext`: Pack focused context around a file + line for agent token budgets.
- `gtslint`: Run structural lint rules against indexed symbols.

This first cut includes Go parsing (`.go`) with deterministic JSON output for agent use.

## Quickstart

```bash
go run ./cmd/gts gtsindex . --out .gts/index.json
go run ./cmd/gts gtsindex . --out .gts/index.json --once-if-changed
go run ./cmd/gts gtsindex . --incremental --watch --interval 2s --out .gts/index.json
go run ./cmd/gts gtsindex . --incremental --watch --poll --interval 2s --out .gts/index.json
go run ./cmd/gts gtsmap --cache .gts/index.json --json
go run ./cmd/gts gtsfiles . --sort symbols --top 20
go run ./cmd/gts gtsstats . --top 15
go run ./cmd/gts gtsdeps . --by package --focus internal/query --depth 2 --reverse
go run ./cmd/gts gtsbridge . --top 20
go run ./cmd/gts gtsgrep 'function_definition[name=/^Test/,start>=10]' --cache .gts/index.json
go run ./cmd/gts gtsgrep 'method_definition[receiver=/Service/,signature=/Serve/]' --cache .gts/index.json --count
go run ./cmd/gts gtsdiff --before-cache before.json --after-cache after.json --json
go run ./cmd/gts gtsrefactor 'function_definition[name=/^OldName$/]' NewName . --callsites --write
go run ./cmd/gts gtschunk . --tokens 500 --json
go run ./cmd/gts gtsscope cmd/gts/main.go --line 300 --cache .gts/index.json --json
go run ./cmd/gts gtscontext cmd/gts/main.go --line 120 --tokens 600 --json
go run ./cmd/gts gtslint . --rule 'no function longer than 50 lines'
go run ./cmd/gts gtslint . --rule 'no import fmt'
```

## Selector syntax (`gtsgrep`)

Pattern format:

```text
<kind>[filter1,filter2,...]
```

Examples:

- `function_definition[name=/^Test/]`
- `method_definition[name=/Serve/,receiver=/Service/]`
- `type_definition`
- `*[file=/handlers\/.go$/,start>=20,end<=200]`

Supported filters:

- `name=/regex/`
- `signature=/regex/`
- `receiver=/regex/`
- `file=/regex/`
- `start>=N`, `start<=N`, `start=N`
- `end>=N`, `end<=N`, `end=N`
- `line=N` (line contained inside symbol span)

Supported kinds in this version:

- `function_definition`
- `method_definition`
- `type_definition`

## Current scope

- Go-only structural extraction (`go/parser`-backed).
- Cache format: JSON (`.gts/index.json` by default).
- Incremental cache reuse based on file size + mtime metadata.
- Event-driven watch mode via `fsnotify` with polling fallback (`--poll` to force polling).
- Change reporting and CI exit signaling via `--report-changes` and `--once-if-changed`.
- Deterministic ordering for files and matches.
- File discovery (`gtsfiles`) supports language/symbol filters and density-based sorting.
- Stats (`gtsstats`) summarizes symbol kinds, language breakdown, and top files by symbol density.
- Dependencies (`gtsdeps`) summarizes import graph shape with top incoming/outgoing nodes and focus traversal (`--depth`, `--reverse`).
- Bridges (`gtsbridge`) summarizes cross-component internal edges and external dependency pressure.
- Structural diff detects symbol additions/removals/modifications and import changes.
- Structural refactor (`gtsrefactor`) supports AST-aware declaration renames plus same-package callsite updates.
- Chunking (`gtschunk`) emits AST-boundary units with per-chunk token budgeting.
- Scope resolution (`gtsscope`) reports in-scope imports, package symbols, and local declarations for Go files.
- Context packing returns focus symbol, imports, bounded source snippet, and related symbols.
- Structural linting (`gtslint`) supports max-lines rules and import bans like `no import fmt`.
- Index parsing is parallelized by default; set `GTS_INDEX_WORKERS` to tune worker count.
- File scanning filters to parser-supported extensions during walk to reduce indexing overhead.

## Next increments

- Add parser-backed adapters for gotreesitter language modules.
- Expand selector grammar (field filters, boolean filters).
- Extend `gtsrefactor` beyond same-package callsites.
- Expand `gtsscope` beyond Go with parser-backed symbol tables.
