# Ticket status vocabulary

| Status | Meaning |
| --- | --- |
| `needs-triage` | Raw incoming report needs evaluation |
| `needs-info` | A consequential fact or decision is missing |
| `ready-for-agent` | Scope and acceptance criteria are executable |
| `ready-for-human` | Human-only work is required |
| `wontfix` | Explicitly declined work |
| `done` | Implementation and relevant checks are complete; candidate evidence is recorded |

Categories are `bug` and `enhancement`. A local file stores these as fields, not remote labels. `done` does not imply merged, published, or archived. Tickets created by a reviewed breakdown are already agent-ready and need no second triage pass. Loom's preparation and native-turn states remain separate.
