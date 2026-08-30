# Measuring Skill Usefulness

How to measure whether the `lodestone` skill actually helps an agent.

The system under test is **the judgment guidance in SKILL.md plus the embedder**, not the Go implementation.
Go is only the harness that runs the code.

**No evaluation harness needs building.** The adopted benchmark suites ship their own evaluation code.
What remains to build is task packages and the significance arithmetic.

| Document | Contents |
|---|---|
| [skill-evaluation.md](skill-evaluation.md) | The experiment design for measuring whether the skill helps: five arms, quadrant decomposition, regression mechanisms, effect size and N |
| [benchmarks.md](benchmarks.md) | The external benchmark survey: the adopted suites, the rejected ones, and why |

## Adopted benchmarks

| Measures | Tool | Agent |
|---|---|---|
| The whole skill | [SkillsBench](https://github.com/benchflow-ai/skillsbench) — `skill_lift = with − without` | Required. Expensive |
| The embedder | [Agent Retrieval Bench](https://huggingface.co/datasets/eyuansu71/agent_retrieval_bench) — `arb` CLI, BM25 and RepoMap baselines | Not required |
| Description retrieval quality | [SKILLRET](https://github.com/ThakiCloud/SKILLRET) — a corpus of 6,660 skills | Not required |

All three are Apache 2.0.
`environment/skills/` in a SkillsBench task package is where `lodestone/` goes.

## goldmine

`references/toolkit/harness/cmd/goldmine` mines duplicate-pair candidates from the commit history of a Go repository.
It finds commits where a human judged functions to be actual duplicates and merged them, and reconstructs the pair **as of the commit just before** that merge.
Its output becomes the gold answer that a SkillsBench task's `verifier/test_outputs.py` checks against.

```sh
# from the skill directory
go run -C references/toolkit/harness ./cmd/goldmine -repo /path/to/repo -min-lines 6 -min-overlap 0.5
```

It emits three signals.

| Signal | Rationale |
|---|---|
| `inline` | The body of a deleted function overlaps a surviving one. The author judged the two interchangeable |
| `merge` | One new function absorbed the bodies of several deleted functions |
| `extract` | One new helper took the lines removed from several functions |

Similarity is Jaccard over normalized body lines, **intentionally lexical and intentionally not the similarity the skill computes.**
Building gold with the same function under test would make the benchmark grade itself.

The output is candidates, not labels.
The `verdict` field is filled by a human.

Two cautions.

- `-min-lines` must equal the skill's `MIN_LINES`.
  Otherwise pairs the skill excludes by design are counted as misses.
- The `-min-overlap` default is 0.5 and is deliberately permissive: genuine duplicate groups can overlap well below intuition (one real group measured 0.57), and the human `verdict` pass exists to discard the false positives a low bar admits.

## Running the measurement

Steps 1–3 require building nothing.

1. Fetch SkillsBench and confirm its pipeline runs locally on the bundled tasks.
2. Measure the description's retrieval quality with SKILLRET.
3. Measure the embedder alone with Agent Retrieval Bench. If it cannot beat the BM25, RepoMap, and embedding-model baselines, arm D has nothing to show.
4. Build the task packages. goldmine output becomes the verifier's gold, and [toolkit/testdata/prompt.txt](toolkit/testdata/prompt.txt) is the task prompt each condition receives.
5. Put per-task with/without scores into the four quadrants and run a paired significance test.

## What is already established

- A short history can put every mined pair in one merge commit: adjacent functions in one file sharing a name prefix, which `grep` finds instantly.
  Such pairs are correctly labeled but make no discriminating task — the skill exists for cross-package pairs that name-based search misses.
  Check what a mined set actually contains before building tasks from it.
- The harness that measures a skill already exists.
  SkillsBench accepts your own tasks via `--tasks-dir` and fixes the verifier contract.
- SWE-Atlas cannot serve as the task suite.
  48 of its 70 refactoring tasks name the target symbols outright, and most of the rest still name the function.
- SmellBench's guided 0.65 versus targeted 0.92 is the evidence that localization has room to win.

## Expectations

SkillsBench reports a mean lift of +16.2pp for curated skills, with large domain variance.
Healthcare is +51.9pp; **Software Engineering is lowest at +4.5pp.**

This skill is in the SE domain, and its shape matches the negative result of adding procedure documents to an agent that already has the tools.
**Plan N around +4.5pp** — at that effect size, task count decides significance.
There is a real possibility the conclusion is "does not help", and that too is a valid result.
