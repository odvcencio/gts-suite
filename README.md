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
- `gtsrefs`: Query indexed call/reference occurrences by symbol name or regex.
- `gtscallgraph`: Traverse resolved call graph edges from matching roots.
- `gtsdead`: List callable definitions with zero incoming call references.
- `gtsquery`: Run raw tree-sitter S-expression queries across files.
- `gtsmcp`: Run MCP stdio server exposing structural tools to AI agents.
- `gtsdiff`: Compare structural changes between two snapshots.
- `gtsrefactor`: Apply structural declaration renames (dry-run by default).
- `gtschunk`: Split code into AST-boundary chunks for retrieval/indexing.
- `gtsscope`: Resolve symbols in scope at a file + line.
- `gtscontext`: Pack focused context around a file + line for agent token budgets.
- `gtslint`: Run structural lint rules against indexed symbols.

This cut includes generic tree-sitter parsing for languages that ship a `TagsQuery`, with deterministic JSON output for agent use.

## Quickstart

```bash
go run ./cmd/gts gtsindex . --out .gts/index.json
go run ./cmd/gts gtsindex . --out .gts/index.json --once-if-changed
go run ./cmd/gts gtsindex . --incremental --watch --subfile-incremental --interval 2s --out .gts/index.json
go run ./cmd/gts gtsindex . --incremental --watch --poll --interval 2s --out .gts/index.json
go run ./cmd/gts gtsmap --cache .gts/index.json --json
go run ./cmd/gts gtsfiles . --sort symbols --top 20
go run ./cmd/gts gtsstats . --top 15
go run ./cmd/gts gtsdeps . --by package --focus internal/query --depth 2 --reverse
go run ./cmd/gts gtsbridge . --focus internal/query --depth 2 --reverse
go run ./cmd/gts gtsgrep 'function_definition[name=/^Test/,start>=10]' --cache .gts/index.json
go run ./cmd/gts gtsrefs ParseConfig . --cache .gts/index.json
go run ./cmd/gts gtscallgraph main . --depth 2
go run ./cmd/gts gtsdead . --kind callable
go run ./cmd/gts gtsquery '(function_declaration (identifier) @name)' . --count
go run ./cmd/gts gtsmcp --root .
go run ./cmd/gts gtsgrep 'method_definition[receiver=/Service/,signature=/Serve/]' --cache .gts/index.json --count
go run ./cmd/gts gtsdiff --before-cache before.json --after-cache after.json --json
go run ./cmd/gts gtsrefactor 'function_definition[name=/^OldName$/]' NewName . --callsites --cross-package --write
go run ./cmd/gts gtschunk . --tokens 500 --json
go run ./cmd/gts gtsscope cmd/gts/main.go --line 300 --cache .gts/index.json --json
go run ./cmd/gts gtscontext cmd/gts/main.go --line 120 --tokens 600 --semantic --semantic-depth 2 --json
go run ./cmd/gts gtslint . --rule 'no function longer than 50 lines'
go run ./cmd/gts gtslint . --rule 'no import fmt'
go run ./cmd/gts gtslint . --pattern ./rules/no-empty-func.scm
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

- Multi-language structural extraction via gotreesitter tag queries (explicit core queries + inferred query fallback for additional grammars).
- Go import extraction is preserved for dependency analysis.
- Cache format: JSON (`.gts/index.json` by default).
- Incremental cache reuse based on file size + mtime metadata.
- Event-driven watch mode via `fsnotify` with polling fallback (`--poll` to force polling).
- Watch mode supports changed-file updates with optional sub-file incremental parsing (`--subfile-incremental`).
- Change reporting and CI exit signaling via `--report-changes` and `--once-if-changed`.
- Deterministic ordering for files and matches.
- File discovery (`gtsfiles`) supports language/symbol filters and density-based sorting.
- Stats (`gtsstats`) summarizes symbol kinds, language breakdown, and top files by symbol density.
- Dependencies (`gtsdeps`) summarizes import graph shape with top incoming/outgoing nodes and focus traversal (`--depth`, `--reverse`).
- Bridges (`gtsbridge`) summarizes cross-component internal edges, external dependency pressure, and focus traversal.
- Structural diff detects symbol additions/removals/modifications and import changes.
- Structural refactor (`gtsrefactor`) supports AST-aware declaration renames plus same-package and module cross-package callsite updates.
- Raw structural query (`gtsquery`) supports full tree-sitter patterns/captures across indexed files.
- MCP server (`gtsmcp`) exposes `gts_query`, `gts_refs`, `gts_context`, `gts_scope`, `gts_deps`, `gts_callgraph`, and `gts_dead` via stdio JSON-RPC.
- Reference lookup (`gtsrefs`) surfaces `reference.*` tags extracted during indexing.
- Call graph and dead-code primitives (`gtscallgraph`, `gtsdead`) resolve call edges from indexed references.
- Chunking (`gtschunk`) emits AST-boundary units with per-chunk token budgeting.
- Scope resolution (`gtsscope`) reports in-scope imports, package symbols, and local declarations for Go files.
- Context packing supports spatial mode and semantic mode (`gtscontext --semantic`) with configurable call-depth traversal (`--semantic-depth`).
- Structural linting (`gtslint`) supports built-in rules plus query-file patterns (`--pattern rule.scm`).
- Index parsing is parallelized by default; set `GTS_INDEX_WORKERS` to tune worker count.
- File scanning filters to parser-supported extensions during walk to reduce indexing overhead.

## Roadmap Status

- Phase 1 complete: raw tree-sitter query surface (`gtsquery`) is shipped.
- Phase 2 complete: references are indexed and exposed through `gtsrefs`, `gtscallgraph`, and `gtsdead`.
- Phase 3 in progress:
  - Shipped: `gtscontext --semantic` follows call dependencies from the focus symbol with configurable depth.
  - Next: add type dependency pulls and tighter relevance weighting across imports/types/calls.
- Phase 4 in progress:
  - Shipped: `gtslint --pattern path.scm` for query-file-based structural lint rules.
  - Next: add a built-in starter query pattern pack and richer pattern metadata.
- Phase 5 in progress:
  - Shipped: changed-file watch updates now run through incremental watch apply and maintain per-file parse trees.
  - Next: improve diff/edit application for more append/EOF cases and add persistent warm watch-state hydration.
- Phase 6 in progress:
  - Shipped: MCP stdio server command (`gtsmcp`) with tool calls for query/refs/context/scope/deps/callgraph/dead.
  - Next: add MCP wrappers for refactor/lint/chunk and richer schema/streaming diagnostics.
