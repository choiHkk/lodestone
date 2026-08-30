# Installing the lodestone skill

First-time setup on a new machine. Skill-managed models, tools, caches, and build products land outside the repository under `CACHE_DIR` (see Local files below); install the system prerequisites below separately. The skill's own setup does not need `sudo`.

Budget roughly 20 minutes and 5 GB of disk during the default Qwen setup, most of it building Swift dependencies and the Metal library; about 2 GB is retained once the source snapshot and download caches (`sources/`, `uv-cache/`, `huggingface/`) are removed as Local files describes. Granite is an optional additional model with much smaller weights.

## Contents
- Requirements
- Install the default Qwen profile
- Install the optional Granite profile
- First run with the bundled Go adapter
- Local files
- Troubleshooting
- Working on the skill

## Requirements

This skill runs only on Apple Silicon macOS. Inference uses MLX with Metal shaders and has no CPU or CUDA fallback here.

| Requirement | Needed for | Check |
|---|---|---|
| macOS 14 or newer, Apple Silicon | MLX Metal inference | `sw_vers -productVersion`; `uname -m` prints `arm64` |
| Xcode with Command Line Tools | Swift runtime and Metal library | `xcode-select -p` |
| Go 1.26 or newer | Harness and bundled Go adapter | `go version` |
| Swift 6.1 or newer | MLX runtime | `swift --version` |
| Node.js with npm | jscpd lexical detector | `npm --version` |
| `git` | Fetching Swift dependencies and the Granite converter | `git --version` |
| `python3` | Converter syntax checks and the evaluation scripts | `python3 --version` |
| `uv` | Model download and conversion | `uv --version` |
| Network access to Hugging Face | One-time source model download | — |
| Network access to GitHub | Swift dependencies and the optional Granite converter | — |
| Network access to the npm registry | One-time local jscpd install | — |
| Network access to PyPI | Converter environments via `uvx`, and the no-Xcode Metal fallback | — |

A full Xcode install is preferred because the Metal shader compiler ships with Xcode; without it, `make build` falls back to the prebuilt Metal library from the pinned `mlx-metal` wheel. The Makefile uses `/Applications/Xcode.app/Contents/Developer` automatically when Xcode is installed there. For another location, set `DEVELOPER_DIR` for the command or select that developer directory with `xcode-select`.

Install ordinary prerequisites if missing:

```sh
brew install go node uv
```

## Install the default Qwen profile

Run these commands from `references/toolkit/` inside the skill directory — `skills/lodestone/` in a checkout, or the same path under the plugin's directory in `~/.claude/plugins/` for a plugin install. Completed steps are reused.

```sh
GOMAXPROCS=2 make model
make build
make smoke
make doctor
```

The first command downloads the pinned Qwen3 0.6B source and converts it to affine MLX 8-bit weights with group size 64. `smoke` expects a normalized 1024-dimensional response. `doctor` verifies the Qwen repository and revision manifest, model, runtime, and Metal library.

## Install the optional Granite profile

Build the shared runtime only once. Add Granite with:

```sh
GOMAXPROCS=2 make model EMBEDDING_MODEL=granite
make smoke EMBEDDING_MODEL=granite
make doctor EMBEDDING_MODEL=granite
```

Granite uses its separately pinned converter and model directory. Its smoke test expects 384 dimensions. Installing Granite does not change the default; commands without `EMBEDDING_MODEL` continue to use Qwen3.

## First run with the bundled Go adapter

The default run uses Qwen3. `PATTERNS` are Go package patterns resolved inside `REPOSITORY`, and the first invocation also installs jscpd from the npm registry:

```sh
make analyze REPOSITORY=/path/to/repository PATTERNS='./internal/...' LIMIT=10
```

Select Granite for a run with:

```sh
make analyze EMBEDDING_MODEL=granite \
  REPOSITORY=/path/to/repository PATTERNS='./internal/...' LIMIT=10
```

Dupl runs in process as a Go library.

The embedding runtime and jscpd are not restricted to Go. Automatic extraction, mapping, AST evidence, and fusion for another language require a corresponding adapter. Until one exists, use native code search to select source units and follow the language-neutral workflow in [SKILL.md](../SKILL.md).

## Local files

Every generated artifact lands outside the skill directory, under `CACHE_DIR` — by default a `_workspace/.cache/lodestone/` directory created as a sibling of the repository clone (for a plugin install, a sibling of the plugin directory); override it per make invocation with `CACHE_DIR=<path>`. The skill directory itself stays sources-only:

| Path | Contents |
|---|---|
| `models/Qwen3-Embedding-0.6B-8bit-mlx/` | Default converted Qwen weights. |
| `models/granite-embedding-97m-multilingual-r2-8bit-mlx/` | Optional converted Granite weights. |
| `sources/` | Source snapshots used for conversion. |
| `swift-build/` | Swift dependencies, executable, and Metal library. |
| `uv-cache/`, `huggingface/` | Converter and download caches. |
| `jscpd/` | Local jscpd install. |
| `embeddings/` | Model-specific persistent embeddings. |
| `detectors/` | Generated detector reports. |
| `bin/` | The built `code-similarity` analyzer and `golangci-lint`. |
| `pycache/` | Converter syntax-check bytecode. |

Source snapshots and download caches can be removed deliberately after setup if disk space matters. They are needed only for another conversion; converted models and `.complete` markers must remain for analysis.

## Troubleshooting

**`EMBEDDING_MODEL must be qwen3 or granite`.** Use the exact selector. A weight path is not a profile selector.

**MLX Swift build artifacts are missing.** Run `make build`; it orders the executable and shader build correctly.

**Metal shaders come from the prebuilt wheel instead of compiling.** Without `xcrun metal`, `make build` silently uses the pinned `mlx-metal` library. To compile the shaders yourself, point `DEVELOPER_DIR` at a full Xcode developer directory or change the selected developer directory.

**Source snapshot revision does not match the pinned one.** The pin was changed after the snapshot was downloaded. Remove the named `sources/` directory and rerun `make model` to re-download at the new pin.

**Model directory exists but is incomplete.** A prior conversion left a directory without `.complete`. Inspect and remove that exact directory deliberately; the Makefile refuses to overwrite it.

**Qwen converter cannot import `mlx_lm`.** Run `make model`, which creates the pinned environment with `uvx`; do not invoke `scripts/setup/convert_qwen.py` in system Python.

**Granite converter cannot import `mlx_raclate`.** Run `make model EMBEDDING_MODEL=granite`; do not invoke `scripts/setup/convert_granite.py` in system Python.

**The report shows jscpd as `skipped`.** The detail column names the path that was not executable — under the bundled `make analyze` this only happens when `--jscpd` was pointed somewhere wrong, because the Makefile installs jscpd before running. Fix the path instead of treating the missing retriever as a clean result.

**Inference is slow or the machine is unresponsive.** Lower `BATCH`, raise `PAUSE_MS`, or narrow `PATTERNS`. Granite uses less memory but remains opt-in.

## Working on the skill

From `references/toolkit/`:

```sh
make check
```

The check runs golangci-lint, jscpd against the skill source, syntax checks for the three converter files, Go tests, and Go vet. Runtime changes also require both smoke and doctor pairs from [runtime.md](runtime.md).
