---
name: lodestone
description: Find duplicated or near-duplicate logic, clones, and parallel implementations across a codebase — code that does the same thing under different names, in any programming language — by ranking candidate source-unit pairs with local embeddings plus lexical and structural detectors. Use it when two modules, providers, or services may be doing the same work, or when validation, lifecycle, cleanup, or error handling may have drifted between similar implementations. It surfaces and classifies candidates; it does not diagnose isolated code smells, prove equivalence, or render the refactoring verdict.
---

# Lodestone

Use the coding agent's native symbol, text, call-site, and structural searches first. Use this skill when semantic similarity can bring together relevant source units that those searches may miss. It augments the agent's search and judgment; it does not replace them.

The method is language-neutral. A source unit can be a function, method, type or class implementation, test case, or another coherent code region selected by the agent or a language adapter. The Go code in this skill is the execution harness for orchestration, caching, fusion, and reports; it does not define the target repository language. The harness, the Swift runtime, the setup and evaluation scripts, and the evaluation test data all live under `references/toolkit/`.

This workflow ranks source-unit pairs. It can surface duplicated behavior and parallel implementations whose differences reveal inconsistent validation, cleanup, timeout, buffering, or error handling. It cannot find a defect with no useful comparison unit, prove behavioral equivalence, or decide whether an abstraction is worthwhile.

Treat the ranked pairs as leads. Confirm them by reading the code, callers, and tests. jscpd and any applicable structural detector provide independent evidence, while the target language's compiler, linter, and tests remain the validation tools.

Read only the reference relevant to the current task. For using the skill:

| Reference | Covers |
|---|---|
| [references/install.md](references/install.md) | First-time setup on a new machine, prerequisites, troubleshooting. |
| [references/retrieval.md](references/retrieval.md) | Why ranks — not raw metrics — are fused, plus the embedding scheme, evidence signals, and exclusions. |
| [references/report.md](references/report.md) | Report structure and what each part is for. |
| [references/runtime.md](references/runtime.md) | Shared Swift MLX runtime, model selection, JSONL protocol, and cache separation. |

For working on the skill itself rather than using it:

| Reference | Covers |
|---|---|
| [references/architecture.md](references/architecture.md) | Harness layout, language adapters, dependency direction, conventions. |
| [references/detectors.md](references/detectors.md) | jscpd and dupl integration and the behaviors that fail silently. |
| [references/qwen3.md](references/qwen3.md) | Default Qwen3 pin, conversion, Swift path, pooling, and validation. |
| [references/granite.md](references/granite.md) | Optional Granite pin, conversion, custom ModernBERT path, and validation. |
| [references/evaluation.md](references/evaluation.md) | How to check an encoding, threshold, or model against a known answer, including model tradeoffs and the limits of the initial evaluation. |
| [references/usefulness.md](references/usefulness.md) | Whether the skill helps an agent at all: adopted external benchmarks, the goldmine history miner, and the measurement plan. |
| [references/skill-evaluation.md](references/skill-evaluation.md) | The experiment design for measuring skill usefulness: conditions, quadrant decomposition, effect size and N. |
| [references/benchmarks.md](references/benchmarks.md) | The external benchmark survey: adopted suites, rejected ones, and why. |

## When to use it

Use it when two or more areas appear to perform parallel work but differences in names, modules, or syntax make comparison difficult. Useful review questions include:

- whether providers or services implement the same contract with different validation or error mapping;
- whether connection setup, cancellation, timeout, and cleanup follow the same ownership rules;
- whether equivalent streaming paths add avoidable buffering, copying, or termination states;
- whether tests repeat a protocol fixture or intentionally remain independent;
- whether a high-level responsibility has drifted across components even when the source units are not merge candidates.

For the bundled Go adapter, set `INCLUDE_TESTS=1` when the question concerns test fixtures or test helpers. For other languages, include or exclude tests through the selected source scope or adapter.

Do not run it when ordinary search already identifies the relevant source units, or when the question concerns one isolated unit, dead code, a security flaw, or a performance hotspot without a comparison point. Use dedicated analysis for those tasks. A repository-wide embedding scan is the most expensive mode; use it only when narrower scopes do not answer the question.

## Boundaries

- Treat every retriever as candidate retrieval, never proof of duplication, inconsistency, or poor quality.
- Report what the tool surfaced separately from what code inspection established. A high rank is an observation about retrieval, not a design conclusion.
- Confirm candidates against structure, callers, data ownership, error paths, side effects, concurrency, and tests before recommending a merge or consistency change.
- Keep semantic results report-only until a labeled evaluation set specific to the repository under review establishes a reliable threshold. The bundled planted set checks the skill's own settings, not any particular target, so do not quote the cosine threshold as though it were calibrated.
- Preserve intentional independence. Protocol conformance tests, provider adapters, and boundary-specific validation may need similar code so one shared helper cannot hide the same defect on both sides.
- Treat Go as the implementation language of the harness, not a restriction on analyzed code. Python remains limited to model conversion under `references/toolkit/scripts/setup/` and the evaluation scripts under `references/toolkit/scripts/eval/`; Swift to MLX inference.
- State which source-unit extractor and retrievers were available for the target language. Missing structural evidence is absent evidence, not a clean result.
- Reuse a completed local model and build. Do not download or convert again unless the pinned inputs changed or validation fails.
- Keep local model weights, caches, and build products untracked.

## Choose an execution mode

### Language-neutral review

For any programming language, first infer the repository's languages. Prefer an applicable detector provided by this skill, then one already configured or installed in the target repository or environment. Do not restrict the choice to Go. If no suitable detector is available, ask before installing jscpd or another tool or downloading dependencies.

1. Use native search, symbols, call sites, and the language's established tools to choose coherent source units and the smallest relevant scope.
2. Run lexical and structural detectors that understand the language. jscpd supports multiple languages; a language-specific AST detector is optional.
3. Send the same coherent source units to the local embedding runtime for semantic retrieval.
4. Compare retriever ranks only after their locations resolve to the same source units. If normalization is unavailable, report each source separately instead of presenting a false fused rank.
5. Read the code and classify each candidate before changing it.

The MLX runtime accepts source text independent of language. Direct JSONL use is documented in [references/runtime.md](references/runtime.md).

### Bundled Go adapter

The current `make analyze` adapter automates Go package discovery, function extraction, Go AST evidence, jscpd, dupl, semantic retrieval, and fusion. Resolve the skill directory once and use its toolkit path for every `make` in this document. Before the first run on a machine, run `make -C <toolkit> doctor`; if it fails, stop and ask the user before any setup, because `make analyze` and `make smoke` trigger both the model download (~2 GB) and the runtime build (~20 minutes) when missing, while `make model` and `make build` each trigger their own half.

```sh
make -C /path/to/lodestone/skills/lodestone/references/toolkit analyze \
  REPOSITORY=/path/to/repository \
  PATTERNS='./internal/alpha,./internal/beta' \
  OUTPUT=/tmp/lodestone-report.md LIMIT=15
```

Always write the report to a file and read it in slices: at the default `LIMIT=50` it carries 50 pairs with two function bodies each, which is far too large to take into context whole.

The available retrievers are:

1. **jscpd** finds lexical copy/paste clones. The bundled adapter limits it to Go; jscpd itself supports multiple languages.
2. **dupl** finds structural Go clones. It is a Go-adapter signal, not a requirement of the language-neutral method.
3. **semantic** retrieves similar source units with the selected local MLX model and is independent of the source language once units have been supplied. Qwen3 0.6B is the default. Select Granite explicitly when its smaller memory footprint or higher throughput matters, including on a thermally constrained machine, rather than narrowing the scope below what the question requires. Granite's code-to-code cosine scores occupy a more compressed range, so give `Separation` more weight than the raw cosine there. Qwen3 also accepts a retrieval task through `INSTRUCTION`, applied to every unit so both sides of a pair are encoded alike. Change it when the review question is not duplicate retrieval, and record the task alongside any reported score.

The Go adapter normalizes findings to unordered function pairs and fuses them by reciprocal rank fusion, `sum(1 / (k + rank))` with `k = 60` by default (`RRF_K`). The method can fuse any number of normalized retrievers. Only positions are combined. Raw token counts, linter thresholds, and cosine similarity are never mixed into one weighted score because they share no scale. A retriever that contributes no pairs is a valid outcome and does not block the others; an inapplicable or missing retriever must be reported as absent.

The current Go adapter excludes generated files and explicitly ignored functions, then lists both kinds of exclusion in the report:

- **Generated files.** Machine-written Go is duplicated by construction, so keeping it would fill the report with pairs nobody can act on. `INCLUDE_GENERATED=1` keeps them.
- **Functions marked `similarity:ignore`.** Putting that marker in a Go function's doc comment retires a candidate the repository has already judged acceptable.

## Choose the scope

Do not start repository-wide when the task or a conventional detector already points at specific code. Choose the smallest module, directory, package, or file set containing both sides of the comparison:

- for current edits, the modules or packages holding the changed files;
- for a detector pair, the scope holding both source units;
- for a cross-provider or cross-service comparison, only those components;
- for Go test review, the production package and its tests with `INCLUDE_TESTS=1`;
- the whole repository only for an explicitly repository-wide review, or when no narrower evidence exists.

Budget roughly 20 source units per second for Qwen3 on a first encode and pass a generous command timeout; the embedding cache persists what a run managed to encode, so a retry resumes rather than restarts. If the scope holds fewer than two eligible source units, add the nearest comparison component. If the scoped result does not answer the question, widen once to the nearest domain subtree. Widen to the whole repository only when that comparison is genuinely relevant. The embedding cache persists across scopes, so repeated source text does not require new inference.

## Read the report

### Retrievers table first

Every automated report opens with a table naming each retriever, its status, and how many pairs it contributed.

| Status | Meaning |
|---|---|
| `ran` | Executed. Zero pairs means it found nothing. |
| `provided` | A pre-existing jscpd report was reused instead of a fresh run. |
| `skipped` | Never executed; the detail column says why. Under the bundled adapter both detectors are provisioned automatically. |
| `failed` | Executed and errored. The detail column carries the message. |

`ran` with zero pairs and `skipped` look alike in the candidate list but mean opposite things. A `skipped` or `failed` row means the fused order covers fewer sources than it appears to; treat that retriever's evidence as absent, not as evidence of absence, and say so in any conclusion drawn from the report.

### Signals per candidate

| Column | Range | What it measures | How to read it |
|---|---|---|---|
| `RRF` | small positive | Fused rank across retrievers | Comparable only against other rows in the same report. Never an absolute score. |
| `Sources` | list | Which retrievers found the pair, and at what rank | Evidence of retrieval agreement. Multiple sources improve confidence that the pair is similar, not that changing it has higher design value. |
| `Semantic` | `[-1, 1]` | Cosine between the two normalized source-unit embeddings | Rendered as `-` when the pair came only from a conventional detector and was not retained by the semantic prefilter — `MIN_SCORE`, the per-unit top-10 candidate cap, or `CANDIDATE_POOL` (the JSON `semanticScore` is `0` there). Qwen3's input may include the configured task prefix. |
| `Separation` | 0 upward | How far the pair stands above the next candidate in the same source unit's list, in that list's standard deviations | Measured before the score floor (`MIN_SCORE`), against the unit's whole distribution. High means the unit has one standout neighbor; near zero means a plateau where many units look equally close. Comparable across units in a way `Semantic` is not. |
| `Structure` | 0–1 | Cosine over adapter-provided statement-type counts | Order-blind. In the bundled adapter this is Go AST evidence; it is unavailable without a structural adapter. |
| `Sequence` | 0–1 | One minus normalized edit distance over adapter-provided statement types | Keeps order and nesting. In the bundled adapter `0.0000` also appears when a Go function exceeds the node cap. |
| `Size` | 0–1 | Smaller statement count over larger | An upper bound on `Sequence` under the bundled adapter's normalized edit-distance measure. A low ratio means one unit does substantially more than the other. |
| `Calls` | 0–1 | Jaccard overlap of called names | Low overlap with high structure suggests parallel shapes over different collaborators. |
| `Locality` | enum | `cross-package`, `same-package`, `same-file` | Not a quality signal. It only breaks ties, preferring the distant pair, because near-neighbor duplication is usually already visible to whoever edits the file. |

### Patterns worth recognizing

- **High `Structure`, low `Sequence`.** The same statement mix arranged differently. Common false positive; the functions often share a skeleton but not behavior.
- **High scores, one source.** Plausible, but supported by one retrieval method. Inspect it without treating source count as a design priority.
- **`dupl` or `jscpd` only, `Semantic` at `-`.** Structurally or lexically alike but not retained by the semantic prefilter. Worth reading; often boilerplate.
- **High `Semantic`, low `Structure` and `Sequence`.** The interesting case this skill exists for: possibly similar responsibility with different implementation. Also where the model is most likely to be wrong.
- **High `Semantic`, low `Size`.** Usually one source unit is the other plus retries, logging, or error handling. The embedding sees the shared purpose; the size ratio shows they are not interchangeable.
- **High `Semantic`, low `Separation`.** The pair scores well but so does everything else that unit sees, which is the shape a caller and its callee produce, or a family of units built from one template. Read the unit's other candidates before treating the pair as distinctive.
- **Moderate `Semantic`, high `Separation`.** The unit has one neighbor that stands clear of the rest even though the absolute score is unremarkable. This can reveal a useful counterpart for a low-scoring unit that sorting by `RRF` alone would place late.

The report holds only the top `LIMIT` pairs by `RRF`, and it does not list a unit's other candidates — to use `Separation` for discovery, raise `LIMIT` (and `CANDIDATE_POOL` with it; each unit keeps at most its top ten neighbors) or re-run scoped to the unit's own module. And "high" or "low" in these patterns always means relative to the other rows of the same report, never an absolute band: `MIN_SCORE` already removed everything below the floor, so every surviving `Semantic` value is high on the model's own scale.

### Known biases and blind spots

- Qwen3 receives the same configured task prefix on both sides of every pair; Granite receives raw source. The Qwen3 task biases retrieval toward matching inputs, outputs, and side effects, but either model can still rank topical, lexical, or structural resemblance highly. Confirm responsibility and failure semantics in code instead of reading the rank as an equivalence judgment.
- RRF favors agreement. Lexical or structural clones found by multiple retrievers can outrank a useful semantic-only pair, so inspect the semantic source ranking when syntax differs strongly.
- `Locality` prefers cross-package pairs only as a tie-breaker. Cross-package does not mean more important, and same-package does not mean safe to merge.
- Whole-unit embeddings can dilute a small problematic region, and `MAX_TOKENS` truncates long units. Use chunking only after a known long unit gives reason to inspect it.
- The current Go adapter removes comments before embedding. Other adapters must define comment handling explicitly. Domain intent must still come from the reviewer, interfaces, callers, tests, and documentation.
- Pairwise analysis cannot detect isolated complexity, leaks, unsafe APIs, dead code, or hotspots by itself.

## Confirm and classify a candidate

Each candidate should carry both source units, capped to keep the report usable. The bundled Go report uses `SOURCE_LINES` lines per function. Use carried source for triage, then open the files, callers, and tests before acting on a candidate.

Compare at least:

- syntax-tree and control-flow shape when the language adapter provides them;
- called dependencies, functions, and methods;
- input, output, and error behavior;
- state changes, I/O, and concurrency boundaries;
- existing tests and known callers.

Classify the result before editing:

1. **Merge candidate:** behavior, ownership, and failure semantics match closely enough that one implementation or helper reduces maintenance cost.
2. **Consistency defect:** the functions should remain separate, but an invariant such as validation, cleanup, timeout, error mapping, or buffer ownership has drifted.
3. **Intentional parallelism:** similar structure is required to preserve independent protocol or boundary verification.
4. **Incidental similarity:** the pair shares vocabulary or shape but no maintenance responsibility.

When the evidence does not separate a merge candidate from intentional parallelism, treat the pair as intentional and report only the invariant drift.

Do not assume the correct improvement is a shared function. A smaller fix may align an invariant, remove one redundant buffer, reuse a test setup helper, or clarify an API boundary. Avoid introducing a shared module whose only purpose is to eliminate a few test lines.

Report retriever ranks and embedding scores separately from inspection evidence. State which retrievers ran, what the pair classification was, and what concrete invariant justified any change.

## Bundled Go adapter settings

These variables configure the current automated Go adapter. They are not requirements of the language-neutral review method.

| Variable | Default | Effect |
|---|---|---|
| `REPOSITORY` | required | Target repository root. |
| `PATTERNS` | `./...` | Comma-separated Go package patterns for the bundled adapter, resolved inside `REPOSITORY`. The default scans the whole repository — the most expensive mode; name packages explicitly. |
| `OUTPUT` | stdout | Report path. |
| `FORMAT` | `markdown` | `markdown` or `json`. |
| `EMBEDDING_MODEL` | `qwen3` | `qwen3` or `granite`; selects the pin, converter, model path, cache, dimension, and operational score default together. |
| `INSTRUCTION` | model-specific | Retrieval task applied to every source unit. Qwen3 wraps it as `Instruct: <task>\nQuery:`; Granite declares empty prompts and ignores it. Empty embeds raw source. |
| `MIN_SCORE` | `0.65` for Qwen3; `0.80` for Granite | Minimum semantic cosine retained. Operational prefilter, not an accuracy threshold. Qwen3 scores move with `INSTRUCTION`, so reevaluate the two together. |
| `LIMIT` | `50` | Candidates in the report. |
| `CANDIDATE_POOL` | `300` | Semantic candidates kept before fusion. Must be at least `LIMIT`. |
| `RRF_K` | `60` | Reciprocal rank fusion constant. |
| `MIN_LINES` | `6` | Shortest function considered; also forwarded to jscpd as its `--min-lines`. |
| `MIN_NODES` | `3` | Fewest Go AST statements considered. |
| `MIN_TOKENS` | `50` | Minimum jscpd clone size. |
| `DUPL_THRESHOLD` | `150` | Minimum Go dupl clone size in tokens. |
| `SOURCE_LINES` | `80` | Function lines carried into the report; `0` omits bodies. |
| `INCLUDE_GENERATED` | unset | Keep machine-generated Go files, which are otherwise skipped. |
| `INCLUDE_TESTS` | unset | Keep Go `_test.go` functions. Synthetic test-main files in the Go build cache remain excluded. |
| `CHUNK_TOKENS` | unset | Token window size; unset embeds whole Go functions. |
| `SPLIT_AFTER` | unset | Only window Go functions above this token count. |
| `BATCH` | `8` | Encoded source units or chunks per inference batch. |
| `MAX_TOKENS` | `768` | Token limit per encoded source unit or chunk. |
| `PAUSE_MS` | `100` | Pause between uncached inference batches. |
| `JSCPD_REPORT` | unset | Reuse an existing jscpd report instead of running it. |
| `EMBEDDING_CACHE` | model-specific under the cache directory's `embeddings/` | Persistent embedding cache. The Makefile changes it with `EMBEDDING_MODEL`; the cache directory is `CACHE_DIR` in [references/install.md](references/install.md). |
| `DETECTOR_WORK_DIR` | the cache directory's `detectors/` | Where generated detector reports land. |

Whole source units are the default for the bundled adapter because its Go evaluation found that 50-, 100-, and 150-token windows increased inference work and produced less stable rankings. Do not assume that result transfers unchanged to every language adapter.

The embedding cache is persistent, and the Swift runtime starts only when a cache miss requires inference. Keep batches at eight and a short pause between uncached batches on a shared workstation.

## Prepare or repair the runtime

First-time setup and troubleshooting are in [references/install.md](references/install.md). Reuse a valid local model and runtime. Diagnose before repairing:

```sh
make -C /path/to/lodestone/skills/lodestone/references/toolkit doctor
```

`doctor` is side-effect-free. `make smoke` verifies one embedding response but shares `analyze`'s setup prerequisites, so on an unprovisioned machine it too triggers the download and build — run it only after `doctor` passes or setup is authorized.

Those commands check Qwen3 by default. Append `EMBEDDING_MODEL=granite` to check the optional Granite path.

Run `make build` only when the runtime is missing or stale. Run `GOMAXPROCS=2 make model` only when the selected model is genuinely missing or invalid and a network download is within the task's authorization. Do not overwrite an incomplete conversion. Model selection, conversion invariants, and direct JSONL use are covered in [references/runtime.md](references/runtime.md).
