# Qwen3 default profile

Read this file before changing the Qwen conversion path, Swift embedder integration, pooling, dimension, or default settings.

## Pin and conversion

The default profile pins `Qwen/Qwen3-Embedding-0.6B` at revision `97b0c614be4d77ee51c0cef4e5f07c00f9eb65b3`. `scripts/setup/convert_qwen.py` uses `mlx-lm` 0.31.3 to produce affine MLX 8-bit weights with group size 64. `conversion.json` records the repository, revision, converter version, quantization mode, bit width, and group size.

The converter rejects a source `model_type` other than `qwen3` and refuses to overwrite an existing output directory. Run it through the Makefile, from `references/toolkit/`, so the package version, cache directories, and completion marker remain pinned:

```sh
GOMAXPROCS=2 make model
```

## Swift path

The shared runtime routes `model_type: qwen3` through MLXEmbedders' registered Qwen3 implementation. It uses the model-provided last-token pooling strategy and returns a normalized 1024-dimensional vector.

The profile applies its instruction to every unit, so both sides of a pair are encoded the same way. `INSTRUCTION` carries the retrieval task and this profile wraps it as `Instruct: <task>\nQuery:`; an empty value disables the template and embeds raw source. Adjust it when the review question is not duplicate retrieval, and record the task alongside any quoted score because the setting changes the score.

Do not restore the query/document split, where only one side carried the prefix and the two directed scores were averaged; encoding one side of a symmetric comparison differently from the other is what that arrangement got wrong. The wording is worth choosing rather than accepting, because it changes how far an answer separates from its runner-up. The default here was measured on a small planted set, so treat it as provisional and re-measure before trusting it on another repository. [evaluation.md](evaluation.md) covers the check.

## Operational defaults

`MIN_SCORE=0.65` is the Qwen operational prefilter. It is not calibrated as a duplicate threshold — a genuine pair can score below it. Override it for the repository being inspected when a labeled comparison shows that the cutoff hides useful pairs.

The default cache is `$(CACHE_DIR)/embeddings/qwen3-embedding.gob`. Converting the pinned inputs produces a 633,152,542-byte weight file. Weight size is an artifact check, not part of the model contract.

## Validation

```sh
make smoke
make doctor
```

`smoke` must produce a normalized 1024-dimensional embedding. Shared runtime changes also require the Granite smoke test documented in [runtime.md](runtime.md).
