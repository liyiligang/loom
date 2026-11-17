---
name: grilling
description: Grill the user relentlessly about a plan, decision, or idea. Use when the user wants to stress-test their thinking, or uses any 'grill' trigger phrases.
---

Stress-test the requested scope until consequential open decisions are resolved. Read the [project workflow](../../../docs/agents/workflow.md) and reuse recorded answers. In a Loom Grill preparation stage, follow the stage protocol: record focused questions for the form, write the receipt, and finish the native turn instead of waiting for a second interactive interview.

Work the tree in **rounds**. The **frontier** is every consequential decision whose prerequisites are settled. Ask a manageable batch, with a recommendation for each; questions that depend on an unanswered prerequisite belong to a later round. Wait for required decisions, and continue independent fact-finding while answers are pending.

Format a round like so:

```
❓ **Q1** - **<question title>**: <question body, might be multiple paragraphs, including multiple choices>

➡️ <your recommended answer>

---

❓ **Q2** - **<question title>**: <question body, might be multiple paragraphs, including multiple choices>

➡️ <your recommended answer>
```

Word each question so "yes" accepts your recommended answer.

Each round the user answers reshapes the tree: settled decisions push the frontier outward and unblock questions that depended on them. Recompute the frontier and ask the next round. A question whose answer depends on another question still open in this round belongs to a _later_ round, not this one.

Find environment facts directly. Delegate substantial independent exploration when authorized and available; a lookup does not require a subagent. Only questions downstream of missing facts wait. Ask the user for consequential preferences and product decisions that the evidence cannot settle.

The interview is complete when the requested scope has no consequential unanswered decision. Record decisions and remaining nonblocking assumptions. Reuse the user's existing authorization for subsequent work; obtain any genuinely missing approval at the concrete action it governs.
