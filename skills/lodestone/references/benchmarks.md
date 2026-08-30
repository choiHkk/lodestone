# External Benchmark Survey

**The harness already exists. What remains is task packages and the significance arithmetic.**

There is a dedicated benchmark that measures a skill as a skill (SkillsBench), one that measures the embedder as a retriever (Agent Retrieval Bench), and one that measures the retrieval quality of a skill description (SKILLRET).
All three are Apache 2.0 and ship their evaluation code.

The system under test splits into three pieces, and each piece has its own tool.
Try to measure all three with one bench and none of the numbers stays readable.

| Measures | Tool | Needs an agent |
|---|---|---|
| The whole skill (document + embedder) | SkillsBench | Yes. Expensive |
| The embedder | Agent Retrieval Bench | No. Deterministic and cheap |
| Description retrieval quality | SKILLRET | No. Cheap |

## Contents
- SkillsBench — the dedicated skill benchmark
- Agent Retrieval Bench — the embedder alone
- SKILLRET — description retrieval quality
- Evidence that there is room to win
- Rejected as a task suite
- Why clone-detection benchmarks cannot be used
- Papers

## SkillsBench — the dedicated skill benchmark

`benchflow-ai/skillsbench`, HuggingFace `benchflow/skillsbench`, Apache 2.0.
87 task packages as of v1.1.

The headline metric is exactly what is needed.

```
skill_lift = with_skills_score − without_skills_score
```

A paired lift: the same agent and model run with and without the skill.
Arms A versus D in [skill-evaluation.md](skill-evaluation.md) are this bench's native structure.

You can plug in your own tasks.

```
tasks/<task-id>/
├── task.md                          the prompt
├── environment/
│   ├── Dockerfile
│   └── skills/                      ← where lodestone/ goes
├── oracle/solve.sh                  reference solution
└── verifier/
    ├── test.sh
    └── test_outputs.py              deterministic grading
```

```sh
# bench is SkillsBench's own CLI
bench eval run --tasks-dir <path> --agent claude-agent-acp --model <model> --sandbox modal
```

Containerization, sandboxing, paired execution, and aggregation are all provided.
The gold pairs `harness/cmd/goldmine` mines become the answers `verifier/test_outputs.py` checks, and `oracle/solve.sh` need only be that pair list.

Three things remain to build or run here.

| Item | Why it remains |
|---|---|
| Task packages | The domain is this skill's own. goldmine output becomes the verifier's gold |
| Quadrant decomposition and significance | `skill_lift` is a net gain. See below |
| Arms C and E | The bench has two conditions, with and without. Express the extra arms as task variants with different `skills/` contents |

### Reporting only net gain is this bench's limitation

`skill_lift` is a net gain, and that is exactly the trap Regression Tax points out.
Regressions can eat 59% of gross gains and a net gain will not show it.

But per-task with/without scores come out, so putting them into the four quadrants and running a paired significance test over them is trivial arithmetic.
No harness to write; only this arithmetic remains custom.

### Set expectations low up front

Curated skills raise mean pass rate by **+16.2pp**.
Domain variance is large — Healthcare **+51.9pp**, **Software Engineering lowest at +4.5pp.**

This skill is in the SE domain, and its shape matches the negative result of adding procedure documents to an agent that already has the tools.
**Expect around +4.5pp, and catching that significantly takes task count.**

## Agent Retrieval Bench — the embedder alone

427 samples, 25 repositories, 6 languages (Python 13, Go 3, Rust 3, TypeScript 3, Java 2, JavaScript 1).
Provides the `arb` CLI with Lexical, BM25, RepoMap, and embedding-model baselines; metrics are MRR, Recall@k, BCY, and selective success@20.

The four tasks are code2test 106, comment2context 80, trace2code 101, and edit2ripple 58.
**`edit2ripple` — finding the files affected by a change intent and file edit — is closest to how duplicate detection is used in practice.**
Not a perfect match: its gold includes callers and tests.

There is an 82-sample no-gold control group.
**Saying "nothing" when there is nothing is an official metric** — the same discipline SKILL.md demands in separating candidates from evidence.

Evaluation runs at a frozen base commit, blocking shortcuts that leak through gold paths, patches, or fix commits.

## SKILLRET — description retrieval quality

A benchmark for retrieving the right skill for a natural-language request from a corpus of 6,660 skills.
Metrics are NDCG@k, Recall@k, and Completeness@k; the evaluation code fetches its data from HuggingFace automatically.

Put this skill's SKILL.md in the corpus and it measures whether requests like "find the duplicates" retrieve this skill.
When an agent never uses a granted skill, arm D degenerates into arm A; this targets that confounder directly, without running an agent.

## Evidence that there is room to win

**SmellBench** ran two instruction settings and grades localization separately from repair.

| Setting | What the prompt gives | Localization accuracy |
|---|---|---|
| Guided | Only the smell kind and the file. The location must be found | 0.65–0.67, flat across difficulty |
| Targeted | Names the exact class and method | About 0.92 |

Test pass rate exceeds 80% in both.
**The agent knows how to fix and does not know where.**
That gap — roughly 0.25 to 0.27 — is the size of the share a retriever can win.

SmellBench itself cannot be the task suite.
It is Python-only, and its seven smells do not include duplicated code.

## Rejected as a task suite

These are task collections, not skill harnesses.
Either our tasks cannot plug in, or the skill would have no room to contribute.

| Bench | Tasks | Rejection |
|---|---|---|
| SWE-Atlas Refactoring | repository refactoring | **48 of 70 tasks name the target files, symbols, signatures, and goal design in the prompt.** Most of the remaining 22 still name the function. Specification implementation, not discovery; nothing for a retriever to contribute |
| SmellBench | code-smell repair | Python-only; no duplicated code in the smell list |
| RefactorBench | 100 multi-file refactorings | Python-only |
| SWE-Refactor | 1,099 pure refactorings | 18 Java projects |
| Multi-SWE-bench | 1,632 issue resolutions, Go included | Duplicate awareness rarely changes the answer |
| SWE-bench Multilingual | 1,100 issue resolutions, Go included | Same as above |
| CoIR code-to-code | similar-code retrieval | Python and C++ only; Go is in no subtask |
| CodeSearchNet | function from docstring | Has Go, but a different task |
| SWE-Explore | ranking issue-relevant regions | Has a direct retriever contract and 84 Go cases, but gold is issue-relevant regions, not duplicate pairs; cannot win as-is |
| CORE-Bench | changed files from an issue | Different task |

The SWE-Atlas numbers were counted directly from the downloaded task set.
Of the 70 refactoring tasks Go is the largest language at 30, with 10 in the deduplication family.
SWE-Atlas itself already runs `claude-code` as an agent through its harbor harness and gives 3 epochs with `-k 3`.

Strip the prompt's pointers and the discovery gap is restored with the graders reused as-is.
But at that moment the score is no longer official, and the tests are written against one refactor shape, so a valid deduplication of a different shape grades as failure.

## Why clone-detection benchmarks cannot be used

| Bench | Language | Scale |
|---|---|---|
| BigCloneBench | Java | ~8M labeled pairs |
| SemanticCloneBench | Java, C, C#, Python | 4,000 manually verified pairs |
| GPTCloneBench | same as above | 37,149 true / 19,288 false |
| POJ-104 / OJClone | C | 104 problems × 500 solutions |
| GoogleCodeJam | Java | 1,669 solutions |

**None has Go.**
And even if one did, their reliability is under attack in the literature.

139 of 179 surveyed papers used BigCloneBench for semantic-clone evaluation where it was unsuitable, and 93% of sampled weak clones were labeled clones despite differing functionality.
The authors recommend restricting BigCloneBench to Type-1, 2, and 3 detection.

Follow-up work in 2026 reports that SOTA semantic clone detectors rely on lexical and structural shortcuts rather than semantic understanding and collapse on distribution-shifted Type-4 instances.
This skill's semantic retriever may sit in the same trap, and that would surface as the difference between arms C and D.

[Go-Clone](https://github.com/wangcong15/go-clone), the prior work on Go clone detection, also had no dataset to use and **built its own from 6,110 commit versions of 48 GitHub projects.**
That is what `harness/cmd/goldmine` does.

## Papers

| Paper | Contents |
|---|---|
| [SkillsBench](https://arxiv.org/abs/2602.12670) | **Adopted.** Paired skill lift, 87 tasks, domain variance with SE at +4.5pp |
| [Agent Retrieval Bench](https://arxiv.org/abs/2607.24882) | **Adopted.** 427 samples, no-gold control, grading arbitrary retrievers |
| [Skill Coverage](https://arxiv.org/abs/2606.20659) | A test-sufficiency metric for skills: how much of a skill a task set covers |
| [LH-Bench](https://arxiv.org/abs/2603.22744) | An ablation rerun without SKILL.md; the effect differs per harness |
| [SkillLearnBench](https://arxiv.org/abs/2604.20087) | Continual-learning evaluation of skill creation |
| [SmellBench](https://arxiv.org/abs/2606.05574) | Guided 0.65 versus targeted 0.92; localization separated from repair |
| [SWE Atlas](https://arxiv.org/abs/2605.08366) | Rejected. Prompts name the target |
| [SWE-Explore](https://arxiv.org/abs/2606.07297) | Retriever output contract and nDCG@500 |
| [SWE Context Bench](https://arxiv.org/abs/2602.08316) | Measuring the ceiling first with no-context versus oracle |
| [CORE-Bench](https://arxiv.org/abs/2606.11864) | Building ground truth from pre-patch checkouts |
| [LocAgent](https://arxiv.org/abs/2503.09089) | Ablation design that removes tools one at a time |
| [BigCloneBench misuse](https://arxiv.org/abs/2505.04311) | Misuse in 139 of 179 papers; 93% of weak clones mislabeled |
| [Are We There Yet?](https://arxiv.org/abs/2606.25272) | Shortcut reliance of semantic detectors |
| [GPTCloneBench](https://arxiv.org/abs/2308.13963) | A recipe for growing Type-4 pairs with an LLM |
| [CoIR](https://arxiv.org/abs/2407.02883) | Code-retrieval metric conventions |
| [RefactorBench](https://arxiv.org/abs/2503.07832) | Python; agents 22% versus humans 87% |
| [Multi-SWE-bench](https://arxiv.org/abs/2504.02605) | Multilingual issue resolution including Go |
| [SWE-Refactor](https://arxiv.org/abs/2602.03712) | Java; extracting pure refactorings from commits |
| [Just-in-Time Code Duplicates Extraction](https://arxiv.org/abs/2302.03416) | Prior work building duplicate labels from commit history |

SKILLRET ships as a repository rather than a paper — `github.com/ThakiCloud/SKILLRET`.
