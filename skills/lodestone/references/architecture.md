# Harness and adapter layout

How the toolkit is laid out and which dependency directions are legal. Read the section covering the package you are about to change.

## Implementation languages

The implementation lives under `references/toolkit/`; paths in this document and its siblings are relative to that directory. Go implements the execution harness: CLI handling, orchestration, caching, scoring, fusion, and reports. This implementation choice does not restrict the language of analyzed source. Python is limited to the model converters under `scripts/setup/` (covered in [runtime.md](runtime.md)) and the evaluation scripts under `scripts/eval/` (covered in [evaluation.md](evaluation.md)), and Swift is limited to MLX inference under `runtime/Sources/lodestone`.

## Target-language support

The semantic model accepts source text from any programming language. jscpd is also a multi-language lexical detector.

The bundled automated adapter currently discovers Go packages, extracts Go functions, computes Go AST evidence, runs dupl, invokes jscpd with `--format go`, and maps every retriever result back to those functions. It therefore does not collect non-Go jscpd clones. This is an adapter limitation, not the intended scope of the skill.

A new language adapter should supply coherent source units and location mapping. Syntax-tree evidence and a structural clone detector are optional capabilities. Semantic retrieval and jscpd should continue to work when those capabilities are absent, and the report must name unavailable evidence instead of treating it as a clean result.

## Go packages

`harness/cmd/code-similarity` is an entry point and nothing else: it calls `cli.Run` and maps an error to an exit code. No logic belongs there. `harness/cmd/` also holds two evaluation utilities: `goldmine` (mines duplicate-pair candidates from git history; see [usefulness.md](usefulness.md)) and `dumpfuncs` (dumps functions as JSONL for `scripts/eval/rank.py`, reusing `internal/analyze`'s embedded-text representation and generated-file test so the evaluation measures the shipped pipeline).

| Package | Responsibility | Reference |
|---|---|---|
| `harness/internal/cli` | Flag definitions, validation, path resolution, writing the report to stdout or a file. | — |
| `harness/internal/pipeline` | The run: scan, drive the retrievers, embed, rank, fuse, assemble the report result. | [retrieval.md](retrieval.md) |
| `harness/internal/analyze` | Current Go source adapter, semantic ranking, and Go AST evidence. | [retrieval.md](retrieval.md) |
| `harness/internal/detect` | The conventional detectors, normalized to ranked pairs. | [detectors.md](detectors.md) |
| `harness/internal/embed` | The MLX runtime client, the persistent cache, and source-unit encoding. | [retrieval.md](retrieval.md), [runtime.md](runtime.md) |
| `harness/internal/fusion` | Reciprocal rank fusion over ranked pair lists. | [retrieval.md](retrieval.md) |
| `harness/internal/report` | Markdown and JSON output, review evidence attachment. | [report.md](report.md) |
| `harness/internal/gofunc` | Go function extraction for the history miner. | — |
| `harness/internal/gitwalk` | Read-only git plumbing for goldmine. | — |
| `harness/internal/mine` | Duplicate-pair mining from commit history. | [usefulness.md](usefulness.md) |

Dependencies run one way: `cli` → `pipeline` → {`analyze`, `detect`, `embed`, `fusion`, `report`}, and `cli` also imports `report` to write the result. `embed` and `fusion` depend on `analyze` for the `Function` type; `fusion` depends on `detect` for `Pair`; `report` depends on both `analyze` and `fusion`. Nothing depends on `cli` or `pipeline`.

`analyze.Function` is the current Go-specific source-unit representation. Generalizing automatic analysis means replacing that dependency with a language-neutral source-unit contract and keeping Go parsing behind an adapter. Go remains the harness language after that change.

## Conventions

`harness/internal/cli` holds the flag defaults (the profile-dependent `min-score` default lives in `pipeline`). A new setting is added to `pipeline.Options`, bound in `cli.bind`, validated in `cli.validate`, and forwarded by the `analyze` target in the Makefile.

Lint runs with `golangci-lint` at `default: all`, and jscpd runs against the toolkit's own source with a zero threshold. Both must stay clean; the tool is expected to survive its own analysis.

Keep comments that capture a non-obvious invariant or change tool behavior, such as `//nolint:` and `//go:` directives. Put broader rationale in the relevant reference document.
