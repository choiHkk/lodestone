# Measuring Whether the Skill Helps the Agent

The system under test is **the judgment guidance in SKILL.md plus the embedder**, not the Go implementation.
Go is only the harness that runs the code, and the harness language is irrelevant to this question.

"Does a written skill actually make an agent better?" became an independent research topic in 2026, and the experiment designs and failure modes are already organized in the literature.
This document is that organization.

## Contents
- Five arms
- Net gain alone is deceptive
- Three ways a skill hurts
- Skills work by procedural anchoring, not knowledge injection
- A negative result with the same shape exists
- Effect sizes are small; N decides the experiment
- Papers

## Five arms

Regression Tax recommends three conditions — no library, descriptions only, descriptions plus bodies.
Splitting the last into with- and without-embedder arms, and adding a length-matched control to separate the effect of grown context from the effect of the guidance itself, gives five.

| Arm | What it gets | What it isolates |
|---|---|---|
| A | Agent alone | Baseline |
| B | One line: the SKILL.md description | The effect of presence without invocation |
| C | Full SKILL.md, no embedder | The value of the judgment guidance itself |
| D | Full SKILL.md plus the embedder and report | What the embedder earns |
| E | An irrelevant document length-matched to C | The cost of the added context |

C is the crux.
Most of SKILL.md is not embedder usage but judgment discipline — candidates are not evidence, high Structure with low Sequence is a common false positive, confirm through structure, call relationships, error paths, side effects, and tests.
It can change agent behavior with no embedder at all.

**If C equals D, the embedder does not earn its cost.**

## Net gain alone is deceptive

Regression Tax observed 553 gains and 324 regressions across 5,832 runs.
**Regressions cancel 59% of gross gains.**
Reporting mean improvement makes that cost invisible.

Two skills with the same net effect can be entirely different.
One adds without breaking; the other gains only as much as it breaks.

So results are split four ways.

| Quadrant | Without | With |
|---|---|---|
| Gains | fail | pass |
| Regressions | pass | fail |
| Residual failures | fail | fail |
| Retained | pass | pass |

## Three ways a skill hurts

| Mechanism | Observation | Its shape in this skill |
|---|---|---|
| Skill-description osmosis | The description changes behavior merely by sitting in context, without invocation. 14 of 81 OfficeQA-Pro regressions | Why arm B exists |
| Grounding displacement | An invoked skill overrides a correct reading of the input. 59 of 81 | The most dangerous. When the report ranks a pair first, the agent may follow the ranking over its own judgment |
| Verification displacement | The skill suppresses output verification | Appears as merge recommendations without confirmation |

SKILL.md hammering in "the retriever yields candidates, not evidence" is precisely the grounding-displacement defense.
Whether that defense actually works is what gets measured.

## Skills work by procedural anchoring, not knowledge injection

The decomposition in the most recent work attributes 65.7% of skill effect to procedural anchoring and 4.5% to explicit knowledge injection.
Gains concentrate in the execution layer — environment failures drop from 5.3% to 0.2%, output-format mismatches from 7.4% to 3.2%.

Failures come along too.
Misapplication is 10% under the skill condition versus 0.4% under control.
Growing the retrieval pool from 5 skills to 100 collapses actual-use precision from 29.6% to 3.3%.
Correct skill selection was neither sufficient nor necessary for success.

Skills that help compress exploration into a clean procedure, transfer across frameworks, and can be manipulated during execution.
Skills that hurt encode rigid assumptions, carry task-specific detail, and demand mechanical application without context.

## A negative result with the same shape exists

Giving an agent that already held tools via MCP a procedure document for those tools produced no significant improvement.
The structure matches this skill's situation — the agent already has grep and code reading, and the skill adds a document and an embedder.

There is a real possibility the conclusion is negative, and that too is a valid result.

## Effect sizes are small; N decides the experiment

Regression Tax had 3 of 18 conditions survive Bonferroni correction (α=0.0028).
The survivors were +43 to +47 tasks, all on the Claude Code and SpreadsheetBench combination.
Other work reports a mean of +2.8 points on static benchmarks and 3.7 to 6.7 points on industrial workflows.

That is why these papers run at the scale of 5,832 and 7,560 runs.
Catching +3pp with a paired test needs a substantial product of task count and epochs.
Task count decides significance.

## Papers

| Paper | Contents |
|---|---|
| [The Regression Tax](https://arxiv.org/abs/2607.22520) | Three conditions, quadrant decomposition, three regression mechanisms, Bonferroni pass rate |
| [Demystifying Agent Skills](https://arxiv.org/abs/2608.14036) | Procedural anchoring 65.7%, misapplication 10%, precision collapse as the retrieval pool grows |
| [When Skills Don't Help](https://arxiv.org/abs/2605.20023) | The negative result of adding procedure documents to an agent that already has the tools |
| [LLM-Generated Skills for Data Science](https://arxiv.org/abs/2607.07504) | 56 tasks, 7,560 runs; ablates procedures, examples, and reference notes against a token-matched control |
| [Not All Skills Help](https://arxiv.org/abs/2606.15390) | Measuring and repairing skill knowledge |
| [SkillJuror](https://arxiv.org/abs/2606.11543) | How skill composition changes runtime behavior |
| [Procedural Memory](https://arxiv.org/abs/2606.23127) | Control, adaptation, and evaluation of procedural memory |
| [SkillAxe](https://arxiv.org/abs/2606.10546) | Evaluation-driven skill self-improvement |

Anthropic's skill-creator skill, as currently shipped, already provides the plumbing: it spawns with_skill and without_skill in parallel, records total_tokens and duration_ms in timing.json, and aggregates into benchmark.json.
It has no significance testing, so run its output through a paired significance test.
