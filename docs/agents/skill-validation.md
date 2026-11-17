# Skill behavior validation

Use these realistic requests when substantially changing workflow instructions. Give an evaluating agent the request, installed skills, and minimum raw repository artifacts, without the expected result below. Use an isolated fixture and prohibit network writes, production changes, and paid model calls unless specifically authorized. Compare observable artifacts and actions; do not test headings or exact prose.

| Scenario | Request and fixture | Observable result |
| --- | --- | --- |
| Working changes | Review a tracked edit, staged edit, new file, and staged edit reverted in the working tree | All reviewable states are represented; no empty-diff success claim |
| No standalone spec | Review a concrete bug fix with no issue reference | Correctness and standards reviewed; missing spec reported without blocking |
| No remote tracker | Turn an agreed feature into tickets in a repo without tracker credentials | Local files with acceptance criteria and real blocking edges; no setup loop or external write |
| Small change | Fix an understood documentation typo | Focused edit and inspection; no invented spec, interview, or behavioral test |
| Loom preparation | Generate the supplied Grill stage with answered prior questions | Only authorized artifacts and matching receipt; new consequential questions recorded for the form |
| Managed workspace | Implement a ticket in a supplied fixed Loom workspace | Binding retained; no unmanaged reset, worktree creation, merge, or cleanup |
| Source maintenance | Change a canonical script and its reference, then sync | Complete mirror updated; independent mirror edits preserved and reported |

`scripts/tests/` automates Git-scope and synchronization cases. The other cases require instruction-level evaluation when changed; record whether validation was static or actually executed. Real native provider compatibility still requires separately authorized task-level validation.
