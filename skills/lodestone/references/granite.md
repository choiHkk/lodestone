# Granite optional profile

Read this file before changing the Granite conversion path, custom ModernBERT implementation, CLS pooling, dimension, or Granite-specific defaults.

## Pin and conversion

The optional profile pins [`ibm-granite/granite-embedding-97m-multilingual-r2`](https://huggingface.co/ibm-granite/granite-embedding-97m-multilingual-r2) at revision `835ad14087e140460703cf0fae09f97d469d65c2`. The model card is the source for its Apache-2.0 license, architecture, supported languages, and intended retrieval uses.

`scripts/setup/convert_granite.py` uses `pappitti/mlx-raclate` pinned to commit `2a85e79035da8f6fc06cfd865639ed5e5a16cdb5`. It produces affine MLX 8-bit weights with group size 64. `conversion.json` records the model and converter revisions with the quantization settings.

The converter rejects a source `model_type` other than `modernbert` and refuses to overwrite an existing output directory. Run it through the selected Makefile path:

```sh
GOMAXPROCS=2 make model EMBEDDING_MODEL=granite
```

## Swift ModernBERT path

The pinned MLX Swift LM revision does not register `modernbert`, so `runtime/Sources/lodestone/ModernBert.swift` implements the required embedding path locally. It uses fused QKV projection, alternating local and global attention, separate RoPE bases, SiLU gated MLP layers, final normalization, CLS pooling, and float32 L2 normalization.

Quantized `Linear` and `Embedding` modules are installed only where matching scale tensors exist, then every weight is loaded with strict verification. The runtime accepts affine quantization for this path and rejects an unsupported mode.

Truncation preserves the first CLS token and final SEP token. Taking a plain prefix drops SEP on a full-length input and changes the embedding.

Granite receives raw source with no task prefix and returns a normalized 384-dimensional vector.

## Operational defaults

Run every `make` command in this document from `references/toolkit/` (see [install.md](install.md)).

`MIN_SCORE=0.80` prevents Granite's higher cosine baseline from materializing nearly every pair before ranking: on the `harness/internal/embed` self-check (18 functions at the time of writing), 0.65 retained 146 of 153 pairs while 0.80 retained 27. Re-measure it by running the bundled adapter over `harness/internal/embed` at each `MIN_SCORE` value. The value is an operational bound, not a calibrated duplicate threshold.

The default Granite cache is `$(CACHE_DIR)/embeddings/granite-embedding-97m-multilingual-r2.gob`. The converted weight file used for validation is 103,557,490 bytes.

## Local validation

The converted model was checked against its BF16 source, and the Swift path against the Python MLX reference; both agreed at cosine 0.9997 or better across varied inputs, including full 768-token sequences. Granite's resident memory at batch eight is roughly half of Qwen3's, consistent with its far smaller weights (103 MB against 633 MB), and its throughput is several times higher. On a natural-language doc-comment-to-code proxy Granite also clearly beat Qwen3 on Recall@1 and MRR.

That proxy measures natural-language-to-code retrieval, not this skill's code-to-code duplicate discovery, and its corpus does not ship with this repository — treat the comparison as directional, not as numbers to re-quote. It supports choosing Granite for device constraints or natural-language queries but does not justify replacing the Qwen default.

Device constraints are a user-visible signal, not only a benchmark: when the person whose machine runs the scan reports fan noise, thermal throttling, battery drain, or memory pressure, switch that session to Granite instead of narrowing the scope below what the review question needs. The trade is retrieval sharpness, not correctness: Granite's code-to-code cosines sit in a compressed band, so read `Separation` rather than raw rank and keep `MIN_SCORE=0.80`. `BATCH` and `PAUSE_MS` remain the finer levers when even Granite must yield the machine. The direct model comparison is in [evaluation.md](evaluation.md).

```sh
make smoke EMBEDDING_MODEL=granite
make doctor EMBEDDING_MODEL=granite
```

`smoke` must produce a normalized 384-dimensional embedding. Shared runtime changes also require the default Qwen smoke test documented in [runtime.md](runtime.md).
