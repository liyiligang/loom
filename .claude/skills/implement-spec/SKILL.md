---
name: implement-spec
description: "Implement a multi-session spec as a dependency graph, respecting managed workspaces and recording final-candidate verification."
disable-model-invocation: true
---

Read the [project workflow](../../../docs/agents/workflow.md), spec, and tickets. Use the [tracker](../../../docs/agents/issue-tracker.md) only for operations the task needs. The goal is every acceptance criterion implemented and verified on the final candidate; ticket completion does not by itself establish integration.

## Managed execution

When Loom supplies a fixed workspace or preparation protocol, use its stage and ownership rules. Preparation produces only authorized artifacts. In execution, work within the assigned member and report any blocked task that requires another controller-prepared workspace. This skill does not create a development Loop executor or replace Loom's writer admission. Combine branches only through the authorized integration workflow and verify the combined candidate.

## Standalone execution

1. Read the tickets as a task graph. Identify the ready frontier and preserve the spec's acceptance IDs. Use a local file breakdown when no external tracker was selected.
2. Choose sequential implementation by default. Use fresh implementer subagents for independent ready tickets when parallel work is authorized and the harness supports isolated worktrees. Pass spec, ticket, base commit, and workspace pointers instead of duplicating the full context.
3. For authorized parallel work, prepare one branch/worktree per implementer from the pinned integration candidate. Inspect an existing checkout before adopting it; preserve dirty or unrelated changes. A mismatched base requires an explicit reconciliation, not a reset that discards work.
4. Each implementer follows `implement` and reports candidate identity, acceptance coverage, checks, and remaining blockers. Serialize integration changes; update the frontier as dependencies complete. Publishing or opening a PR follows the selected delivery workflow.
5. Review the final combined candidate with `code-review` and run its relevant checks. Resolve actionable findings; stylistic suggestions do not automatically expand the spec. Mark local tickets done with their evidence when their actual criteria pass.
6. Report delivery state and remaining workspaces. Cleanup applies only to owned, idle, clean workspaces whose work is preserved in the chosen integration target and whose removal is authorized.
