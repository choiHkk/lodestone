# Embedding runtime and model selection

The shared Swift MLX process serves both bundled models. Python is limited to model conversion and Go to orchestration, as described in [architecture.md](architecture.md). None of those implementation languages restricts the language of source text sent to the model.

## Select a model

Qwen3 0.6B is the default for this skill's code-to-code retrieval. Run every `make` command below from `references/toolkit/` (see [install.md](install.md)):

```sh
make smoke
make analyze REPOSITORY=/path/to/repository PATTERNS='./internal/...'
```

Granite is opt-in:

```sh
make smoke EMBEDDING_MODEL=granite
make analyze EMBEDDING_MODEL=granite \
  REPOSITORY=/path/to/repository PATTERNS='./internal/...'
```

`EMBEDDING_MODEL` accepts only `qwen3` and `granite`. The Makefile changes the repository pin, converter, local directory, expected dimension, cache path, and operational `MIN_SCORE` default together. Do not select a model by overriding the weight path alone; that makes the report and cache identity unreliable.

Read [qwen3.md](qwen3.md) when changing the default model path. Read [granite.md](granite.md) when changing the optional ModernBERT path. The comparative evidence and its limits are in [evaluation.md](evaluation.md).

## Shared encoding contract

Both bundled profiles encode each source unit once and compare normalized vectors symmetrically. Qwen3 prepends the configured task as `Instruct: <task>\nQuery:` to every unit; an empty `INSTRUCTION` embeds raw source. Granite always receives raw source because its Sentence Transformers configuration declares empty prompts. Do not restore a query/document split that prefixes only one side: in the check recorded in [evaluation.md](evaluation.md), that asymmetric encoding ranked the known code-to-code pair second where every symmetric encoding ranked it first.

The Go adapter removes Go comments before inference. Chunked functions average their chunk vectors and normalize the aggregate again. Model output is normalized before it reaches Go, so the pairwise dot product is cosine.

Qwen3 and Granite use different dimensions and pooling implementations. The Swift executable reads `config.json` and accepts only the registered `qwen3` and `modernbert` model types. A model directory cannot be made compatible by relabeling its profile.

## JSONL protocol

The runtime is a long-lived process because model loading is expensive. Go starts it once and only when a cache miss requires inference.

Start it with `--model`, optionally `--max-batch` and `--max-tokens` (unrecognized flags are rejected), then write one request per line:

```json
{"id":1,"texts":["func add(a, b int) int { return a + b }"]}
```

Each response carries the request id, normalized embeddings, dimension, and inference time. A request with no texts or more than `--max-batch` texts is rejected. Text longer than `--max-tokens` is truncated at the token level; both paths keep the original terminal token — Qwen3 because its last-token pooling reads it, Granite because dropping SEP changes every position's context. Note that `--chunk-tokens`/`--split-after` count Go lexical tokens while `--max-tokens` counts model subwords (typically 1.5–3× more), and Qwen3's instruction prefix is charged against the same budget — keep chunk sizes well below the token limit.

Batch size is capped because attention memory grows with the longest token sequence in a batch. One long source unit in a batch of eight costs as much attention memory as eight long units. The adapter batches eight functions by default and briefly pauses between uncached batches so a shared workstation remains usable.

## Cache isolation

The cache namespace includes the selected profile, model path, token limit, and the size and modification time of `model.safetensors` (falling back to `conversion.json` for a sharded model). The Makefile also chooses a separate cache file for each bundled model. Keep both protections: the namespace prevents an explicitly shared cache file from serving vectors produced by the other profile. The cache key also includes the complete encoded text, so changing Qwen3's instruction produces cache misses for the newly prefixed inputs.

Changing the model, weights, token limit, or another encoding behavior invalidates prior embeddings. Use a new model directory or cache namespace rather than editing conversion metadata to match old artifacts.

## Build and verify

`make build` compiles one Swift executable containing both the high-level Qwen embedder and the local ModernBERT implementation. It then builds the colocated Metal shader library.

Run validation for both profiles after changing shared runtime code:

```sh
make smoke
make doctor
make smoke EMBEDDING_MODEL=granite
make doctor EMBEDDING_MODEL=granite
```

`smoke` expects 1024 dimensions for Qwen3 and 384 for Granite. `doctor` checks the selected profile's completion marker, repository and revision manifest, weights, runtime, and Metal library.

If a converted model directory exists without `.complete`, a conversion was interrupted. The build refuses to overwrite it; inspect and remove it deliberately.

First-time setup and failure modes are in [install.md](install.md).
