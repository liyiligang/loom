---
name: to-spec
description: "Synthesize agreed context into a concise, buildable spec, reusing existing requirement materials and acceptance criteria."
disable-model-invocation: true
---

Read the [project workflow](../../../docs/agents/workflow.md) and [tracker](../../../docs/agents/issue-tracker.md). Synthesize what the conversation and code establish. Reuse an existing requirement spec, confirmed answers, glossary, and relevant design decisions.

For Loom preparation, follow its supplied stage and material paths; preserve stable acceptance IDs and write its receipt. For standalone work, save locally according to the tracker unless external publication is explicitly part of the task. Missing remote configuration does not block writing a local spec.

## Buildable scope

Explore unresolved facts in the code. Record only the decisions needed to implement and verify the agreed behavior. Reuse existing public test boundaries; a question is needed when choosing a boundary or an unresolved product decision materially changes acceptance. Make ordinary implementation choices directly.

A useful spec contains:

- **Problem and outcome:** who needs what behavior and why.
- **Scope and exclusions:** the requested change and relevant limits.
- **Acceptance criteria:** a concise, complete set of observable results, with stable IDs when the project uses them. Use user stories where they clarify distinct flows; length is not a completion criterion.
- **Design decisions:** interfaces, data, failure behavior, and constraints that affect implementation. Reference authoritative design or prototype decisions instead of copying them.
- **Verification:** the public boundaries and scenarios that demonstrate the criteria, including meaningful failure cases.
- **Open points:** consequential unresolved decisions, with reasons. Preserve assumptions as assumptions rather than treating them as approvals.

The spec is complete when an implementer can identify the agreed behavior and how to verify it. Update the existing material rather than producing a second authoritative copy. Preserve current tracker status unless the task authorizes changing it.
