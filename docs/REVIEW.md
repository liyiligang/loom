# First slice review

The installed code-review workflow performed independent Standards and Spec reviews against Git's empty-tree baseline (`4b825dc642cb6eb9a060e54bf8d69288fbee4904`). The repository had no prior commits. The specification was `DESIGN.md`, scoped to the initial foundation and direct Codex increment. Reviews used source inspection; they did not execute agent tasks.

## Standards

No hard `AGENTS.md` violations were found. Project content uses English, and `DESIGN.md` remains version controlled.

The review found two initial correctness defects: unfinished restored items were marked complete and stopped accepting deltas; switching sessions during an awaited submission could erase another session's draft. Follow-up inspection found that a stale snapshot could overwrite final output and terminal restoration could leave expired interactions visible.

These four defects were addressed by native snapshot finality checks, preservation of completed projections and newer prefixes, source-session draft guards, and shared terminal interaction cleanup.

One judgment-call smell remains: state values use strings across the models and UI. Named state types and narrower transition helpers may be useful as the state model grows. This is not a documented-standard violation.

Standards count: zero hard violations, four correctness findings addressed, one optional smell. The most serious issue was restoration dropping or rolling back streamed output.

## Spec

The review found four initial defects: unacknowledged intent could not reconnect to a native turn, approvals were marked resolved before remote confirmation, coalesced refreshes could lose a final notification, and session switching could erase another draft. Follow-up inspection identified a late-notification variant of intent orphaning and the stale-snapshot output race.

The fixes persist a native-turn baseline, correlate identifiable intent under the single-writer constraint, allow late notifications to bind the sole unknown intent, wait for `serverRequest/resolved`, drain pending refreshes, preserve newer output, and guard drafts by source session and request ID.

No unrequested behavior was identified. Claude, nodes, and mDNS remain explicitly deferred. M1 and the direct Codex operation paths are present; M2 task-level validation and M3 coverage remain incomplete. Codex-only support does not complete the first-release objective.

Spec count: four initial findings and two follow-up cases addressed. The most serious issue was losing the relationship between a submitted task and its native execution.

## Verification limit

The findings were addressed in source and followed by build, protocol, and browser checks. Failure-path behavioral coverage remains outstanding as described in `VALIDATION.md`.
