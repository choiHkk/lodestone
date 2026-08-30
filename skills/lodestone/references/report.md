# Report structure

What the bundled Go adapter's `harness/internal/report` package emits and why each part exists. A future language adapter may carry different structural evidence while preserving retriever status and source-unit identity.

## Retriever status

A retriever that ran and found nothing, and a retriever that never ran, produce identical candidate lists. A reader who cannot tell them apart will read a single-source report as if three sources had agreed. Every report therefore states each retriever's status:

| Status | Meaning |
|---|---|
| `ran` | Executed. Zero pairs means it found nothing. |
| `provided` | A pre-existing jscpd report was reused instead of a fresh run. |
| `skipped` | Never executed; the detail column says why. Under the bundled adapter both detectors are provisioned automatically. |
| `failed` | Executed and errored. The detail column carries the message. |

When any retriever is `skipped` or `failed`, the report says so in prose as well, because a reader scanning for candidates will not stop to interpret a table.

Detector detail can carry a tool's multi-line stderr, so it is flattened and its pipes escaped before it enters a Markdown table cell.

## Per-candidate evidence

The current candidate table carries a `#` index, `RRF`, the contributing `Sources` with their ranks, then `Semantic`, `Separation`, Go-derived `Structure`, `Sequence`, `Size`, `Calls`, `Locality`, and a `Functions` locator column; the per-candidate details block repeats the signals and attaches the capped function bodies. The retriever table's pair counts are measured before the report cap, so `ran` with zero pairs still means the retriever found nothing. What each measures and how to read it is in [retrieval.md](retrieval.md), and the reading guidance for a reviewer is in [SKILL.md](../SKILL.md).

Two values read as absent or zero for reasons other than dissimilarity:

- `Semantic` renders as `-` in Markdown (and `0` in JSON) when the pair came only from a conventional detector and was not retained by the semantic prefilter (`MIN_SCORE`, the per-unit top-10, or `CANDIDATE_POOL`).
- `Sequence` is zero when either function exceeded the node cap and was not compared.
- `Separation` is zero for detector-only pairs, where it was never measured.

## Function bodies

Each candidate carries both source units; the current adapter emits Go function bodies capped at a configurable line count, with truncation marked in place. Carrying the text makes triage cheaper; it does not replace opening callers and tests before making a change. A cap of zero omits the bodies when the report must stay small.

Bodies and the sequence score are attached in one pass after fusion and limiting, because both are only worth computing for the candidates that actually reach the report.

## Exclusions

Generated files and functions marked `similarity:ignore` are named in the report with counts, truncated after ten entries. The exclusions are deliberate, but a reader asking why a known file never appears needs to see it stated rather than inferring a bug.

Go test functions are different: they are absent by default but are not enumerated in this section. A bundled-adapter report concerning tests must record that `INCLUDE_TESTS=1` was used.

## Formats

Markdown is the default and is meant to be read. JSON carries the same `Result` structure for programmatic use. The generated report file is not subject to lint or test gates; the `report` package itself is tested like the rest of the harness.

`modelProfile` records which bundled model profile produced the semantic ranks. `embeddingMilliseconds` records inference for the single symmetric encoding pass. It excludes model startup and cache hits.

`Separation` accompanies `Semantic` in the candidate table. `Semantic` says how close a pair is on the model's scale, which differs from unit to unit; `Separation` says how far the pair stood above the next candidate the same unit saw, which is comparable between units. A reader scanning for a unit's genuine counterpart, rather than for the repository's highest scores, sorts by it.
