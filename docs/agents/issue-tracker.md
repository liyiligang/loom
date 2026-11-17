# Issue tracker

The default for standalone work is local Markdown. This enables the installed skills without connecting or publishing to an external service. An explicitly supplied external issue can be read as a source; writing to that tracker requires the user's selected workflow.

## Standalone specs and implementation tickets

- One feature directory: `.scratch/<feature-slug>/`.
- Spec: `spec.md`; tickets: `issues/<NN>-<slug>.md`, one file per independently verifiable slice, numbered blockers-first.
- Ticket fields: `What to build`, `Blocked by`, `Status`, and measurable acceptance criteria. Use the status vocabulary in `triage-labels.md`; completed implementation tickets use `Status: done` with the candidate and verification evidence.
- Comments append under `## Comments`. Refer to files by path and title; ambiguous bare numbers need context.
- Publishing locally means writing the files. Read a ticket directly; work on any ticket whose blockers are done. Preserve parent scope and previously recorded decisions.

## Loom requirements

Loom's supplied material root and catalog identify the canonical requirement and design files under `docs/requirements/<requirement ID>/` in the selected material source. Reuse these files as the spec. The supplied preparation stage controls allowed files and receipts. Generic tickets do not modify Loom's ledger or authorize Loop execution.

Execution task orchestration and native ticket relationships are not implemented. When an authorized standalone breakdown is needed for a Loom requirement, keep task notes alongside its existing material files and reference the confirmed spec and acceptance IDs; do not relocate or duplicate the spec. Follow `workflow.md` for workspace ownership.

## Wayfinding

For a standalone effort, use `.scratch/<effort>/map.md` and one decision ticket per `issues/<NN>-<slug>.md`. Each ticket has `Type: research/prototype/grilling/task`, `Blocked by`, and `Status: open/claimed/resolved`. Claim before working; resolve by recording the answer and a context pointer in the map. These decision statuses are separate from implementation completion. External PRs are not a triage request surface by default.
