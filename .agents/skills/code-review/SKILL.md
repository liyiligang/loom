---
name: code-review
description: "Review committed or working changes for correctness, project standards, and agreement with the requested behavior."
---

Review two axes: **Standards and correctness**, and **Spec**. Read the [project workflow](../../../docs/agents/workflow.md). The user's request can supply the spec; a separate issue tracker is optional.

## 1. Capture the actual scope

Choose the scope from the request:

- **Workspace:** current changes, work in progress, or the review at the end of implementation. Default base is `HEAD`. A recorded task-start commit includes any commits made during the task.
- **Commits:** a branch, PR, or changes since a supplied reference. Pin that reference and compare its merge-base with `HEAD`. Local changes are excluded from this mode and must be disclosed.

Run the bundled helper from its actual installation path:

```sh
node <skill-directory>/scripts/review-scope.mjs --mode workspace --cwd <repository>
node <skill-directory>/scripts/review-scope.mjs --mode commits --base <reference> --cwd <repository>
```

The JSON pins the requested base, merge-base, and head commits. Workspace mode includes the final tracked diff, the index diff, the unstaged diff, and nonignored new files. Inspect all these states: staged edits can remain even after the working file is restored to its original content. Binary new files have size and hashes and require suitable inspection; symlinks are recorded as links. The helper fails on invalid refs, oversized inputs, or a changing snapshot instead of reporting a complete review.

Record `snapshotHash` and give every reviewer the same captured input. An empty scope means no changes to review; it is not evidence that requested work was implemented. Ask about the base only when the desired range remains ambiguous.

## 2. Identify the requested behavior and standards

Prefer the supplied task/spec, then explicit issue references or a matching existing requirement document. In Loom, use the supplied confirmed material source and acceptance IDs. Read an external issue only when needed and available; missing remote access does not block review of local changes. When no spec is available, report that limitation and continue the Standards/correctness axis.

Read relevant `AGENTS.md`, `DESIGN.md`, `CODING_STANDARDS.md`, and `CONTRIBUTING.md` where present. Review observable behavior, error paths, boundary cases, access checks, persistence/recovery, and performance when relevant to the diff. Findings need a concrete failure or documented rule. Style and refactoring suggestions remain optional heuristics, subordinate to project conventions; skip checks already enforced by tooling.

## 3. Review and report

Review both axes against the captured scope. For a substantial diff, use independent Standards and Spec subagents when delegation is authorized and available; small diffs can be reviewed directly. Pass the captured input and source pointers to each reviewer.

- **Standards and correctness:** actionable defects and documented-standard violations, with file/line, consequence, and supporting evidence.
- **Spec:** missing or incorrect requested behavior, scope expansion, and acceptance coverage, quoting the source criterion.

Keep the axes identifiable and label optional suggestions separately from defects. Skip speculative findings. If the spec is missing, say so without claiming a Spec pass.

Before delivery, capture the same scope again. A changed `snapshotHash` invalidates the prior review until affected changes are reviewed. Report the scope, candidate identity, validation performed, and remaining limitations. A commit or working-tree digest identifies the reviewed inputs; this helper does not freeze a live directory or replace Loom's immutable candidate verification.
