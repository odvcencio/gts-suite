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
- `gtsquery`: Run raw tree-sitter S-expression queries across files.
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
go run ./cmd/gts gtsindex . --incremental --watch --interval 2s --out .gts/index.json
go run ./cmd/gts gtsindex . --incremental --watch --poll --interval 2s --out .gts/index.json
go run ./cmd/gts gtsmap --cache .gts/index.json --json
go run ./cmd/gts gtsfiles . --sort symbols --top 20
go run ./cmd/gts gtsstats . --top 15
go run ./cmd/gts gtsdeps . --by package --focus internal/query --depth 2 --reverse
go run ./cmd/gts gtsbridge . --focus internal/query --depth 2 --reverse
go run ./cmd/gts gtsgrep 'function_definition[name=/^Test/,start>=10]' --cache .gts/index.json
go run ./cmd/gts gtsrefs ParseConfig . --cache .gts/index.json
go run ./cmd/gts gtsquery '(function_declaration (identifier) @name)' . --count
go run ./cmd/gts gtsgrep 'method_definition[receiver=/Service/,signature=/Serve/]' --cache .gts/index.json --count
go run ./cmd/gts gtsdiff --before-cache before.json --after-cache after.json --json
go run ./cmd/gts gtsrefactor 'function_definition[name=/^OldName$/]' NewName . --callsites --cross-package --write
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

- Multi-language structural extraction via gotreesitter tag queries (explicit core queries + inferred query fallback for additional grammars).
- Go import extraction is preserved for dependency analysis.
- Cache format: JSON (`.gts/index.json` by default).
- Incremental cache reuse based on file size + mtime metadata.
- Event-driven watch mode via `fsnotify` with polling fallback (`--poll` to force polling).
- Change reporting and CI exit signaling via `--report-changes` and `--once-if-changed`.
- Deterministic ordering for files and matches.
- File discovery (`gtsfiles`) supports language/symbol filters and density-based sorting.
- Stats (`gtsstats`) summarizes symbol kinds, language breakdown, and top files by symbol density.
- Dependencies (`gtsdeps`) summarizes import graph shape with top incoming/outgoing nodes and focus traversal (`--depth`, `--reverse`).
- Bridges (`gtsbridge`) summarizes cross-component internal edges, external dependency pressure, and focus traversal.
- Structural diff detects symbol additions/removals/modifications and import changes.
- Structural refactor (`gtsrefactor`) supports AST-aware declaration renames plus same-package and module cross-package callsite updates.
- Raw structural query (`gtsquery`) supports full tree-sitter patterns/captures across indexed files.
- Reference lookup (`gtsrefs`) surfaces `reference.*` tags extracted during indexing.
- Chunking (`gtschunk`) emits AST-boundary units with per-chunk token budgeting.
- Scope resolution (`gtsscope`) reports in-scope imports, package symbols, and local declarations for Go files.
- Context packing returns focus symbol, imports, bounded source snippet, and related symbols.
- Structural linting (`gtslint`) supports max-lines rules and import bans like `no import fmt`.
- Index parsing is parallelized by default; set `GTS_INDEX_WORKERS` to tune worker count.
- File scanning filters to parser-supported extensions during walk to reduce indexing overhead.

## Next increments

- Phase 1: `gtsquery` raw tree-sitter query engine
  - Add a first-class command to run full S-expression queries with predicates/alternation/quantifiers.
  - Output structured captures (file, range, capture name, node text, node type).
  - Keep `gtsgrep` as a fast ergonomic subset, but treat `gtsquery` as the power surface.
- Phase 2: Definition + reference index model
  - Extend index schema to persist references (`reference.call`, etc.), not just definitions.
  - Add `gtsrefs`, `gtscallgraph`, and `gtsdead` commands.
  - Use references for dead code detection and impact analysis.
- Phase 3: Semantic context packing
  - Add `gtscontext --semantic` that follows call/type dependencies instead of line distance only.
  - Rank/pack related symbols under token budget by dependency relevance.
  - Keep existing spatial mode as a fallback.
- Phase 4: Query-file linting
  - Add lint rule loading from `.scm` query files (`--pattern path.scm`).
  - Keep built-in rules, but allow zero-recompile custom structural rules.
  - Ship starter pattern library (`error-must-be-checked`, `no-deep-nesting`, etc.).
- Phase 5: Sub-file incremental watch mode
  - Use `Tree.Edit()` + incremental parsing for changed ranges.
  - Maintain per-file live trees in watch mode and update symbols/references incrementally.
  - Target near-instant watch feedback on large repositories.
- Phase 6: MCP server
  - Expose suite capabilities as MCP tools (`gts_query`, `gts_refs`, `gts_context`, `gts_scope`, `gts_deps`).
  - Keep CLI UX for humans, but make MCP the primary integration point for AI agents.
  - Add stable JSON contracts and versioned tool schemas.
