# Conventional detectors

How `harness/internal/detect` drives jscpd and dupl, and the behaviors that are easy to get wrong. Read this before changing that package.

## Contract

`detect` returns normalized `Pair` values, never report file paths. Rank fusion therefore never learns how a detector reports its findings, and a detector can be replaced without touching `harness/internal/fusion`.

Each detector's pairs are returned sorted by that detector's own metric, largest clone first. The position in that slice is the rank fusion consumes. Metrics are carried on the pair for the JSON output only and are never compared across detectors.

Pairs that do not resolve to two distinct scanned source units are dropped without consuming a rank, so retained ranks stay dense. The current resolver knows only Go functions, and the bundled adapter also constrains jscpd to `--format go` — hardcoded in the detector, not a Makefile setting (the Makefile's `FORMAT` selects the report format). jscpd itself supports multiple languages, but non-Go collection and mapping require another adapter.

## dupl

Runs in process through `github.com/golangci/dupl/lib` (MIT):

```go
func Run(files []string, threshold int) ([]printer.Issue, error)
```

`printer.Issue` holds `From` and `To` clones exposing `Filename()`, `LineStart()`, and `LineEnd()`.

The library call is deliberate: the alternative is shelling out to `golangci-lint run --enable=dupl` and parsing its human-readable message text, which is produced by golangci-lint's own `fmt.Sprintf` — calling the library removes the formatting step entirely rather than trying to reverse it. No linter binary and no report file take part in a run.

`lib.Run` groups clones and emits them as a circular ring: a group of `k` clones yields `k` issues linked `A→B→C→A`. Files are passed as absolute paths built from the repository root.

dupl is Go AST-based, not token-based. It is **not** a substitute for jscpd. It participates only when the target language and adapter support it.

## jscpd

Runs as a subprocess and supports multiple source languages. Version 5's flags differ from version 4.

Flags that matter:

- `--absolute` is **required**. Without it a report names files by basename alone whenever the run is given subdirectories rather than the repository root, and no pair can be matched back to a source unit. This fails silently: the detector reports a clone, every pair is discarded during mapping, and the report shows `ran` with zero pairs.
- `--mode weak` drops comment tokens as well as whitespace, matching the comment removal the embedding retriever does. Without it, two unrelated functions carrying similar commentary register as a lexical clone.
- `--threshold 100` prevents the duplication percentage from producing a non-zero exit in normal inputs. The harness does not pass `--exit-code`, so merely finding clones does not fail the run.
- Version 5 supports `--silent` and `--no-tips`, but the harness does not need them because it discards detector stdout. `.gitignore` is respected by default; `--no-gitignore` disables it.

The JSON reporter writes `jscpd-report.json` into the `--output` directory. Any leftover report there is removed before the run so a failed detector cannot serve stale findings. After a clean exit an absent report means the detector found nothing, which is not an error; after a tolerated exit code 1 an absent report is treated as a usage or input failure and surfaces the detector's stderr.

An exit code of 1 is tolerated — detector findings are advisory — but only when a report was written; anything higher is a genuine failure. A `--jscpd-report` path that does not exist is likewise a failure, not a clean reuse.

## Directory pruning

`Directories` reduces the scanned file paths to the smallest set of parents covering every file. Handing a lexical detector both a parent and its descendant would make it read the same files twice and report each clone more than once.

Pruning can widen the scope beyond the selected packages when a parent and its children are both in scope. That is harmless: pairs from files outside the scan are dropped during function mapping.

## Failure handling

A detector that cannot run is recorded and skipped rather than failing the analysis, because an empty source is a valid outcome for the remaining retrievers. The status reaches the report so a reader can tell a clean detector from an absent one. See [report.md](report.md).
