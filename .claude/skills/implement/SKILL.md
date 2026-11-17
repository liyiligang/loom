---
name: implement
description: "Implement an understood change from a conversation, spec, or ticket, then verify and review the working changes."
disable-model-invocation: true
---

Implement the requested behavior. Read the [project workflow](../../../docs/agents/workflow.md) for task sizing, verification, delivery, and Loom ownership. Read a supplied spec or ticket; the conversation is sufficient for an understood small change. Fetch external references only through the configured tracker. Ask when scope is materially ambiguous.

Record the starting commit and existing local changes. Use the assigned workspace and preserve unrelated work. Reuse agreed acceptance criteria and test boundaries. Drive `tdd` when requested or when a failing behavioral test is useful; otherwise choose verification appropriate to the change.

Run focused checks while implementing, then the relevant repository checks once the change is stable. Review in **workspace mode** with `code-review`, using the recorded starting commit when the task includes new commits. Include staged, unstaged, and new files. Fix actionable findings and rerun affected checks after fixes.

Report the behavior, checks, and reviewed candidate identity. Commit only when the user requested it or the agreed delivery workflow requires it.
