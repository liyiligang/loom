---
name: ask-matt
description: Find the smallest useful skill or workflow for the current task.
disable-model-invocation: true
---

# Choose a workflow

Read the [project workflow](../../../docs/agents/workflow.md). Before recommending a skill, read its `SKILL.md`; the entries here are routing hints. Preserve existing decisions and the user's requested scope.

| Situation | Route |
| --- | --- |
| Small, understood change | `implement` directly, followed by workspace review |
| A consequential product or design decision is open | `grill-me`, or `grill-with-docs` when durable domain decisions are useful |
| A question needs a runnable answer | `prototype`; capture the question, verdict, and source pointer |
| Agreed feature needs a portable implementation contract | `to-spec`, reusing existing requirement materials |
| Build spans several sessions | `to-tickets`, then `implement` per ready ticket or `implement-spec` for authorized orchestration |
| Incoming raw reports | `triage`; existing agent-ready tickets need no extra triage |
| Hard, intermittent, or unclear failure | `diagnosing-bugs` |
| Large effort with unresolved dependencies between decisions | `wayfinder`, then a buildable spec and tickets when needed |
| Review a branch, PR, or current changes | `code-review`, with an explicit commits or workspace scope |
| Learn from a completed or difficult session | `retro` |
| Survey architecture opportunities | `improve-codebase-architecture` |

Loom preparation already provides requirement/design drafts, focused Grill questions, alignment, confirmation, and a Loop preview. Use its stage protocol and reuse its answers. Managed execution retains Loom's fixed workspace; the standalone worktree orchestration route applies only outside that binding.

## Supporting skills

- `tdd`: a requested or useful behavioral red-green loop at agreed public boundaries.
- `domain-modeling`: sharpen terminology and record consequential domain decisions.
- `codebase-design`: choose module shape, interfaces, and test boundaries.
- `research`: investigate primary sources and save cited findings.
- `pr`: write a reviewable PR body when a PR is part of the delivery workflow.
- `to-questionnaire`: collect answers held by another person.
- `wizard`: guide human-only setup steps.
- `handoff`: transfer context between harnesses, directories, or collaborators using source pointers.
- `teach`, `wait-what`, and `writing-for-agents`: learning, clarification, and instruction writing.

## Context transitions

Continue while current context remains useful. Save decisions and acceptance criteria before handing work to a fresh session. A self-contained ticket makes previous implementation context disposable; use the harness's available context controls rather than assuming `/clear` or `/compact` exists. Read [PHASE-BOUNDARIES.md](PHASE-BOUNDARIES.md) when a transition needs more thought.

## Configuration

The repository's shared tracker, domain, and workflow pointers are in `AGENTS.md`. `setup-matt-pocock-skills` changes that configuration when needed; ordinary implementation and review do not require rerunning setup.
