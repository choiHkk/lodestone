# Lodestone

> **Pulls the code that matters out of the mass — matched by meaning, not by name.**

[![License](https://img.shields.io/badge/License-Apache_2.0-green.svg)](LICENSE)
[![Platform](https://img.shields.io/badge/Platform-macOS_(Apple_Silicon)-black.svg)](#requirements)
[![Agent Skill](https://img.shields.io/badge/Claude_Code-Agent_Skill-blue.svg)](skills/lodestone/SKILL.md)

Lodestone is an Agent Skill that finds code by what it means. It embeds source units with a local model, fuses their semantic ranking with lexical and structural detector rankings, and hands your agent the pairs worth comparing: duplicated behavior, parallel implementations, drifted error handling — the things name-based search walks right past.

A lodestone is a natural magnet: it draws the relevant pieces out of a large mass. That is the whole job. Your agent keeps the judgment.

Everything runs locally. No code leaves the machine.

## Quickstart

Install as a Claude Code plugin:

```
/plugin marketplace add choiHkk/lodestone
/plugin install lodestone@lodestone
```

Or expose the skill directly from a clone:

```sh
git clone https://github.com/choiHkk/lodestone.git
cd lodestone
mkdir -p ~/.claude/skills
ln -s "$(pwd)/skills/lodestone" ~/.claude/skills/lodestone
```

Then run the one-time model and runtime setup. Run it from wherever the skill's files live — this checkout, or for a plugin install the plugin's directory under `~/.claude/plugins/` — or let Claude Code run it for you (after asking) the first time the skill needs it:

```sh
cd skills/lodestone/references/toolkit
GOMAXPROCS=2 make model    # download and convert the pinned embedding model
make build                 # build the MLX Swift runtime
make smoke && make doctor  # verify
```

Details and troubleshooting: [install.md](skills/lodestone/references/install.md).

## Usage

Ask Claude questions where name-based search falls short — the skill ranks candidate pairs and Claude confirms them against the actual code:

- "Are there parallel implementations of this validation anywhere else in the repo?"
- "Find places that duplicate this cleanup logic under different names."
- "Do the Go side and the Python side reimplement the same behavior?"

Or run the bundled Go adapter directly, from the repository root:

```sh
make -C skills/lodestone/references/toolkit analyze \
  REPOSITORY=/path/to/repository PATTERNS='./internal/...' \
  OUTPUT=/tmp/lodestone-report.md
```

Add `EMBEDDING_MODEL=granite` to any command to use the smaller profile.

## What lodestone is not

- It does not hand out verdicts. The report is a ranked list of **candidates**, and the skill trains the agent to confirm every pair through structure, call sites, error paths, and tests before recommending anything.
- It does not diagnose isolated code smells, prove behavioral equivalence, or decide whether an abstraction is worth having.
- It does not replace your agent's native search. It fills the gap where meaning and naming part ways.

## How it works

Three retrievers run over the same source units and their rankings are fused with reciprocal rank fusion:

| Retriever | Finds | Engine |
|---|---|---|
| semantic | units that mean the same thing, whatever they're called | local MLX embedding model |
| jscpd | lexical copy/paste clones | jscpd |
| dupl | structural Go clones | dupl (in-process) |

Two pinned embedding profiles are supported; each is downloaded from Hugging Face and converted locally during setup:

| Profile | Model | Dimensions | When |
|---|---|---|---|
| `qwen3` (default) | Qwen3-Embedding-0.6B, 8-bit MLX | 1024 | best separation |
| `granite` | granite-embedding-97m-multilingual-r2, 8-bit MLX | 384 | smaller footprint, higher throughput, thermally constrained machines |

The method is language-neutral — any language's source units can be embedded and compared — while the bundled automation (package discovery, extraction, AST evidence) currently covers Go; other languages follow the language-neutral workflow in the skill itself. See [architecture.md](skills/lodestone/references/architecture.md).

## Requirements

- macOS on Apple Silicon (the runtime is MLX-based)
- Xcode or Command Line Tools (Metal shader build)
- `go`, `node`, `uv` — `brew install go node uv` (plus `git`, `make`, `python3`, which ship with the Command Line Tools)
- ~5 GB disk during setup; about 2 GB retained (converted model and build artifacts) once the source snapshot and download caches are removed

Generated artifacts (models, binaries, caches) land outside the repository, in a `_workspace/.cache/lodestone/` directory created as a sibling of the clone (override with `CACHE_DIR=<path>` on any make invocation) — the repo stays sources-only, and a full build leaves `git status` clean.

## Repository structure

| Path | Contents |
|---|---|
| [skills/lodestone/SKILL.md](skills/lodestone/SKILL.md) | The skill: when to reach for it and how to judge what it returns. |
| [skills/lodestone/references/](skills/lodestone/references/) | Deep documentation: architecture, retrieval, detectors, models, evaluation methodology. |
| [skills/lodestone/references/toolkit/](skills/lodestone/references/toolkit/) | The implementation: Go harness, Swift MLX runtime, setup and evaluation scripts, test data. |
| [.claude-plugin/marketplace.json](.claude-plugin/marketplace.json) | Plugin manifest for `/plugin` installation. |

## Does it actually help?

Whether the skill actually helps an agent is treated as a measurable question — the experiment design, adopted benchmarks, and expected effect sizes are documented in [usefulness.md](skills/lodestone/references/usefulness.md).

## License

Apache License 2.0 — see [LICENSE](LICENSE).

The skill name was suggested by Claude Code.
