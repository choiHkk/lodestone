# Checking a retrieval setting

The encoding, the score floor, and the model are settings, not facts. Each changes which pairs a report contains, and none can be judged by reading the report alone: a setting that hides a true pair produces a clean, plausible, shorter report.

## Contents
- Method
- The instruction belongs on both sides
- Separation reaches pairs that fused rank does not
- Choosing between the two embedders
- Limits

## Method

One labeled pair catches a setting that is plainly wrong; a handful separates a setting that works from one fitted to a single example. The apparatus ships with the skill: five labeled pairs under `references/toolkit/testdata/pairs/` (already split tune/holdout), a small labeled target repository under `references/toolkit/testdata/target/` holding their donor functions, `scripts/eval/plant.py` to plant the pairs, `scripts/eval/rank.py` to read directed ranks over units dumped by `harness/cmd/dumpfuncs` (which emits exactly the representation the adapter embeds), `scripts/eval/score.py` to score a report, and `scripts/eval/measure.sh` to run the whole loop. From the skill directory:

```sh
references/toolkit/scripts/eval/measure.sh                      # plant, analyze, score against the bundled target
go run -C references/toolkit/harness ./cmd/dumpfuncs -repo /path/to/repo > units.jsonl
python3 references/toolkit/scripts/eval/rank.py --units units.jsonl \
  --runtime /path/to/cache/swift-build/release/lodestone \
  --model /path/to/cache/models/Qwen3-Embedding-0.6B-8bit-mlx \
  --query <unit> --partner <unit>
```

Set `TARGET=` to point `measure.sh` at a repository you labeled yourself.

1. Plant or identify pairs whose answer you know, shaped the way the semantic retriever exists for: same behavior, different implementation, different names, different packages. Confirm the conventional detectors return nothing for them, otherwise the semantic path is not what you are measuring.
2. Split them. Choose settings against one part and report the other, because a default chosen and verified on the same pair says nothing about a second repository.
3. Embed every unit once per setting through the runtime directly. A full adapter run adds fusion and thresholds that obscure what the encoder did.
4. Read the directed rank, not the global one: where the known partner lands among the query unit's own candidates. A global pair list mixes units whose score scales differ.
5. Record the spread and the margin over the runner-up, not only the rank. A setting that orders correctly but compresses every score into a narrow band leaves nowhere to put a threshold.

## The instruction belongs on both sides

Encoding one side of a symmetric comparison differently from the other is what an instruction prefix gets wrong, not the instruction itself. Applied as a query against plain documents, the shipped prefix ranked a known pair second where every symmetric encoding ranked it first. (`rank.py --instruction` reproduces exactly that rejected asymmetric arrangement, which is what makes the comparison runnable; the shipped adapter always encodes symmetrically.)

Applied to both sides, the wording matters: every symmetric encoding ranks the known pair first, so rank stops discriminating and the useful statistic is the margin between the known partner and the runner-up in the candidate list's own standard deviations.

| Instruction, applied symmetrically | Margin / σ |
|---|---|
| None | 0.364 |
| `...semantically equivalent behavior` | 0.816 |
| `...duplicates of it and could be merged` | 0.489 |
| `Retrieve functions with the same inputs, outputs, and side effects` | **1.740** |
| `Retrieve functions that handle HTTP request routing` | 0.677 |

The best instruction separated the answer from its runner-up almost five times as far as none did, and a check that reads only the rank sees none of this. The last row keeps the claim narrow: an instruction about HTTP routing, which describes neither function, still beat no instruction, so part of the effect is the presence of an instruction rather than its content.

## Separation reaches pairs that fused rank does not

Absolute cosine and fused rank order pairs across the whole repository; `Separation` asks how far a pair stood above the next candidate the same unit saw. In one earlier run at `LIMIT=50` (the analyzer default; `measure.sh` now passes 200) against a larger labeled corpus, a planted pair sat 44th of 50 report rows by fused rank and second by `Separation`; `measure.sh` prints the same two columns for the bundled target today. The lowest `Separation` rows in that report were each a caller beside its callee, the pattern [SKILL.md](../SKILL.md) describes.

`Separation` also makes `INSTRUCTION` adjustable when a known target pair is available but a full labeled evaluation set is not. Changing the task shifts every absolute score, so two runs cannot be compared on `Semantic` alone; they can be compared on whether the target pair pulled away from the unit's other candidates.

## Choosing between the two embedders

Two evaluations point in opposite directions. They do not conflict; they measure different retrievals, and the skill performs one of them.

| | Qwen3-Embedding-0.6B | granite-embedding-97m-multilingual-r2 |
|---|---|---|
| Code to code: directed rank of a known partner among 93 units (`rank.py`) | **1 of 93** | 2 of 93 |
| Code to code: score spread / median | **0.4089** / 0.4094 | 0.1941 / 0.7243 |
| Doc comment to code: Recall@1, MRR | 27.6%, 0.429 | **50.6%, 0.637** |
| Throughput, 8-bit weights, batch-8 peak RSS | 22.9 vec/s, 633 MB, ~759 MiB | **117.9 vec/s, 103 MB, ~410 MiB** |

Granite wins the second decisively and is far cheaper to run. It ordered the code-to-code pair reasonably, but its scores spanned 0.19 around a median of 0.724 where Qwen3 spanned 0.41 around 0.409, and a duplicate-candidate report needs a useful boundary between a candidate and the rest of the corpus. On Granite's compressed scale a known pair sits within a few hundredths of unrelated units. Natural-language-to-code search typically relies on top-ranked results rather than the same hard candidate boundary, which is why the two evaluations disagree without either being wrong.

Two consequences for operating both. Select per retrieval rather than per benchmark: prefer the wider spread when the question is which units do the same thing, and prefer Granite when the query is natural language or footprint decides. And treat a threshold as belonging to a model, which is why `MIN_SCORE` defaults with `EMBEDDING_MODEL`; any score quoted in a conclusion has to name the model it was measured under.

## Limits

The code-to-code numbers above come from a small planted set in one Go repository — the doc-comment proxy row comes from a separate corpus that does not ship (treat it as directional, per [granite.md](granite.md)) — and the settings they justify were chosen against part of it. A rank is a single observation that a larger set can reverse. The spreads and margins are computed over a unit's whole candidate list and are the more durable half of the evidence: they describe how an encoder treats a corpus, not how it treats one pair.

Treat every number here as a reason to re-measure rather than as a calibration, and grow the labeled set before trusting a default on another repository. The recorded runs also predate an encoding fix in the runtime (attention-mask and truncation handling), which is one more reason the procedure, not the numbers, is what carries.
