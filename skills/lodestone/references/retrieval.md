# Retrieval and fusion

How candidates are found, ranked, and combined — read it to interpret a report's ranks and signals, and before changing anything in `harness/internal/analyze` or `harness/internal/fusion`.

## Why ranks, never metrics

Retrievers produce candidate pairs on different scales. jscpd counts duplicated tokens across supported languages, a language-specific structural detector uses its own representation, and the embedding model produces a cosine similarity in `[-1, 1]`. The bundled Go adapter uses dupl as its structural detector. None of these scores are calibrated against the others, so combining raw values would silently make whichever scale has the largest numbers dominate.

Reciprocal rank fusion combines only positions:

```
score(pair) = sum over retrievers of 1 / (k + rank)
```

with `k = 60`, following [Cormack, Clarke & Büttcher (2009)](https://dl.acm.org/doi/10.1145/1571941.1572114), whose measurements also motivate the default `k`. A pair found by two retrievers therefore outranks a pair found once, no matter how confident that single retriever was. Agreement between independent methods is the signal; magnitude within one method is not.

A retriever that contributes no pairs is a valid outcome, not a failure. It removes one term from the sum and nothing else.

This also creates a ranking bias: lexical or structural clones supported by multiple retrievers can outrank a useful semantic-only pair. RRF answers "which retrievers agreed", not "which pair has the highest design impact". A reviewer looking for different implementations of the same responsibility must inspect the semantic source ranks as well as the fused order.

## Semantic retrieval

Source units are embedded whole. The current Go adapter supplies functions, but the encoding is not tied to Go. Qwen3 is the default and Granite is optional. Both profiles encode each unit once and use the normalized vector directly on both sides of the comparison. Qwen3 applies the same configured task prefix to every unit; Granite receives raw source.

A pair is scored by the cosine between its two vectors. The Swift runtime returns unit vectors and the Go window aggregator normalizes again after averaging multiple chunks, so the dot product is the cosine in both whole-function and chunked modes.

Qwen3's default instruction biases retrieval toward matching inputs, outputs, and side effects, but it does not establish behavioral equivalence. A high rank from either model can reflect shared vocabulary, topic, or shape rather than the same responsibility. Complementary pipeline stages and architecture relationships remain outside the pairwise method and should be found with symbols, callers, interfaces, and data-flow searches.

The default cosine prefilter follows the selected model: 0.65 for Qwen3 and 0.80 for Granite. Granite's higher value prevents its higher cosine baseline from materializing nearly every pair before ranking: on the `harness/internal/embed` self-check (18 functions at the time of writing), 0.65 retained 146 of 153 pairs while 0.80 retained 27. Neither value is a calibrated duplicate threshold, and a threshold measured under one model must not be quoted under the other.

Whole-unit embedding is the bundled Go adapter default because its 50-, 100-, and 150-token window evaluation produced more inference work and less stable rankings. A new language adapter must validate rather than inherit that conclusion blindly.

The current Go adapter removes inline comments before embedding because prose can dominate code similarity. Lines beginning `//go:` remain because they change compilation, and Go doc comments do not enter function source. Other language adapters must define equivalent handling without assuming Go comment syntax.

Scoring every unordered pair is quadratic. The work is spread over `GOMAXPROCS` workers pulling row indices from a queue rather than taking equal slices, because row `i` compares against `n - i - 1` partners and equal slices would leave the worker holding the low indices running long after the others finished.

The embedding cache is persistent and keyed on the model profile, model path, weight file size and modification time, and token limit. The Swift MLX runtime is started only when a cache miss actually requires inference.

## Evidence that is not retrieval

The current Go adapter provides four descriptive signals that never enter retrieval scores. Other adapters may provide a subset or language-equivalent evidence. Adding these values to retrieval ranking would reintroduce the scale-mixing that RRF exists to avoid.

**Structure** is the cosine over counts of Go AST statement types in the bundled adapter. It is order-blind: two source units built from the same statements arranged differently score near 1.

**Sequence** is one minus the Levenshtein distance over pre-order Go AST statement types, normalized by the longer sequence. It keeps ordering and nesting, so it separates pairs that Structure cannot.

Sequence comparison is quadratic in node count, so it is computed only for the candidates that reach the report, not for every retrieved pair, and functions above the node cap return zero rather than stalling the report. A zero therefore means either "no similarity" or "not compared"; the cap is the only way to tell them apart.

**Size ratio** is the smaller statement count over the larger. Under the bundled adapter's normalized edit-distance definition, it is a provable upper bound on `Sequence`: the best achievable sequence similarity for statement sequences of length `m` and `n` is `min(m,n)/max(m,n)`. A low ratio therefore shows that the units cannot have high ordered statement-sequence similarity, regardless of the embedding score.

**Call overlap** is the Jaccard index over called names.

## Locality

The current Go adapter reports `cross-package`, `same-package`, or `same-file`. A language-neutral adapter can map this to module, namespace, or directory boundaries. Locality is not a quality signal and is not summed into anything.

The preference is a discovery heuristic, not evidence that a cross-package pair should share an abstraction. Package boundaries, dependency direction, and independent protocol contracts can make the distant pair less suitable for merging than a same-package pair.

## Exclusions

The following exclusions describe the bundled Go adapter. Test functions are skipped unless `INCLUDE_TESTS=1` is set. This omission is not listed alongside generated files and explicitly ignored functions in the report, so record whether tests were included when the review concerns Go test code.

Generated files are skipped. Machine-written Go is duplicated by construction, so including it fills the report with pairs nobody can act on. Detection uses `ast.IsGenerated`, which requires parsing with `parser.ParseComments` and follows the convention at <https://go.dev/s/generatedcode>, plus a looser check for markers that generators such as easyjson emit without following that convention. The looser check risks excluding a handwritten file that merely mentions those words, which is why every exclusion is named in the report.

Functions with fewer statements than the node threshold are skipped. A line-count minimum alone lets through a one-statement function whose signature is spread over several lines, and those match each other constantly without ever being worth merging.

Functions whose doc comment contains `similarity:ignore` are skipped, so a repository can retire a duplicate it has already judged acceptable without that pair returning in every later report.

The generated-file and `similarity:ignore` exclusions are named in the report; the test and node-threshold exclusions are not, so record them yourself when they matter. An exclusion nobody can see is indistinguishable from a bug.

## Separation

Cosine is most useful within one source unit's candidate list and is not directly comparable across units with different score distributions. A unit whose best genuine counterpart scores 0.62 sits below another unit's unremarkable 0.85, so ordering every pair on one absolute scale buries the first. Candidates are therefore ranked within each unit and fused by position, and `Separation` reports how far a pair stood above the next candidate in its unit's list, in that list's own standard deviations. It is measured before the score floor because the removed tail gives the distribution its shape. With a known target pair, `Separation` can also compare instruction variants without requiring a full labeled evaluation set: the task may shift every absolute score, but the pair's margin over the unit's other candidates remains visible.

The encoding, the score floor, and the model are settings and not facts about retrieval. [evaluation.md](evaluation.md) covers how to check one against a known answer.
