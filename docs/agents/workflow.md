# Agent workflow

## Choose the smallest useful route

| Work | Route | Completion evidence |
| --- | --- | --- |
| Small, understood change | Implement directly; review the working changes | Requested behavior and relevant checks |
| Feature with consequential open decisions | Explore, ask focused questions, record scope and acceptance criteria, implement and review | Agreed criteria and matching validation |
| Work spanning several sessions | Reuse or write a spec, split self-contained tickets with blocking edges, implement the ready frontier | Every criterion covered on the final combined candidate |
| Hard or intermittent bug | `diagnosing-bugs` | A reproduction signal, fix, and regression evidence |

Use `tdd` when requested or when a failing behavioral test is the useful next step. Reuse existing public test boundaries; ask about a boundary when choosing it changes coverage or architecture. Documentation and low-impact appearance changes need suitable inspection rather than tests that merely repeat their implementation. A spec, ticket, or interview is useful only when it resolves uncertainty or makes a handoff executable.

Read a skill's `SKILL.md` before using it. When the harness provides a native Skill tool, use it; otherwise read the file and its applicable references directly. Slash commands and context controls are harness-specific: use available equivalents, and preserve decisions in files before a context transition. Confirmed scope, answers, and test boundaries remain valid until new evidence changes them. Ask for missing consequential decisions; complete already-authorized work without repeating approval requests.

## Loom-managed work

Recognize a Loom task by its supplied workflow/stage/request identity, material root, source hashes, confirmed snapshot, or fixed session/workspace binding. Use the supplied context; a matching path or branch prefix alone does not establish ownership.

- **Preparation:** follow the supplied stage protocol and write its actual artifacts and receipt. Preparation ends at confirmed documents and a Loop preview. Record Grill questions for Loom's form instead of starting a second interactive interview. Preserve acceptance IDs and submitted answers. The receipt and state ledger belong to Loom; generic specs or ticket status do not advance them.
- **Materials:** the selected material source is independent of the execution branch. During a preparation stage, revise only the files that stage authorizes. For execution, use the confirmed versions supplied by Loom and record their hashes with verification evidence. Preserve existing requirement/design documents instead of creating a competing spec under `.scratch/`.
- **Workspaces:** Loom manages branch-group membership, workspace preparation, fixed session directories, and writer admission. Work within the assigned member. Request the required controller action when another workspace is needed. Standalone skill worktree creation, resetting, merging, and cleanup do not apply to a managed binding.
- **Delivery:** record the reviewed candidate commit, or a working-tree snapshot hash while it is uncommitted, together with checks and acceptance coverage. After combining branches, verify the combined candidate. A native turn completing does not mark a requirement integrated. Integration, publishing, and archival use the user's authorized controller workflow.

Automatic development Loop orchestration remains planned. These instructions adapt existing skills to managed sessions; they do not introduce an executor.

## Standalone work

The conversation can supply a sufficient spec. Capture the starting commit and existing local changes, preserve unrelated work, and review the actual working changes before delivery. Commit when requested or required by the agreed delivery workflow. A review without a separate spec reports that limitation and still checks correctness and documented standards.

For multi-session work outside Loom, use isolated worktrees only when parallel implementation is authorized and available. Record each branch's base and delivery target. Keep worktrees with dirty files or unintegrated work until their preservation and cleanup are explicitly resolved.

## Skill maintenance and checks

`.agents/skills/` is the canonical source, including references, scripts, and assets. `.claude/skills/` is its generated installation mirror. Edit the canonical files, then run:

```sh
node scripts/skills.mjs sync
node scripts/skills.mjs check
```

The tool uses the frontend's existing `yaml` dependency; install it with `npm --prefix web ci` when missing. `scripts/skills-lock.json` records the last synchronized files. Sync refuses independently changed mirror files, preserves unowned files, and removes only unchanged, previously generated files that lost their source. Check detects drift, invalid metadata, missing real reference links, and inconsistent native invocation policies. Preserve explicit-only invocation settings and third-party credits when updating skills. New binary assets are copied byte-for-byte; text line endings are normalized for cross-platform checks.

Run `./scripts/check.ps1` on Windows or `sh scripts/check.sh` on macOS/Linux for the repository checks. These install dependencies when direct packages are missing or their installed versions differ from the lockfile; use `-InstallDependencies` or `--install` to force a clean install after other lockfile changes. The command checks skills and tooling, runs frontend/bridge tests and builds embedded assets, then checks Go formatting, vet, and tests. CI uses the same commands with dependency installation enabled. Add focused race checks when concurrency changes warrant them.

Tests in `scripts/tests/` exercise real Git review scopes and mirror synchronization. For substantial instruction changes, use the scenarios in `docs/agents/skill-validation.md`; mechanical validation alone does not establish good agent behavior.
