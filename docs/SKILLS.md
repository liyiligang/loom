# Agent skills

Loom uses the released suite from [AI Hero](https://www.aihero.dev/skills), maintained in [mattpocock/skills](https://github.com/mattpocock/skills).

## Installed scope

- 27 released skills, selected from the upstream plugin manifest.
- Project-local installation for Codex and Claude Code.
- Codex files in `.agents/skills/` and Claude Code files in `.claude/skills/`.
- Copies rather than symbolic links for Windows compatibility.
- Installation sources and content hashes recorded in `skills-lock.json`.
- No global skill or plugin installation.

The upstream repository also contains draft and miscellaneous skills. They are outside the released suite and are not installed here. Do not use `--skill '*'` when the intent is to install only the released set.

Initial installation was checked against upstream commit `4ae2317ddb22b046e26455e4f9c9b3aaaf62212d`, whose plugin manifest reports version `1.3.2`. The installer version used was `skills@1.7.6`. The local lockfile tracks installed content; later updates may change these hashes and versions.

## Install in another checkout

Run the following PowerShell commands from the project root. Node.js and npm are needed for the installer; the installed skills are files and do not require a background service.

```powershell
$releasedSkills = @(
    'ask-matt'
    'code-review'
    'codebase-design'
    'diagnosing-bugs'
    'domain-modeling'
    'grill-with-docs'
    'implement'
    'implement-spec'
    'improve-codebase-architecture'
    'pr'
    'prototype'
    'research'
    'retro'
    'setup-matt-pocock-skills'
    'tdd'
    'to-spec'
    'to-tickets'
    'triage'
    'wayfinder'
    'wizard'
    'grill-me'
    'grilling'
    'handoff'
    'teach'
    'to-questionnaire'
    'wait-what'
    'writing-for-agents'
)

npx skills@latest add mattpocock/skills --agent codex claude-code --skill $releasedSkills --copy --yes
```

For an interactive selection instead:

```powershell
npx skills@latest add mattpocock/skills --agent codex claude-code --copy
```

Select the released skills listed above, including `setup-matt-pocock-skills`. Omitting `--global` keeps installation scoped to the project.

Do not install the same suite as both a plugin and file-based skills in one agent. Upstream documents that this makes each skill available twice. Loom uses file-based installation for both agents.

## First use

The installed skills will be available to Codex on the next turn. Start a new Claude Code session in this project if an existing session does not discover the new files.

Before the first engineering workflow, explicitly invoke `setup-matt-pocock-skills`:

- Codex: select the skill or invoke `$setup-matt-pocock-skills`.
- Claude Code: invoke `/setup-matt-pocock-skills`.

This is a separate, interactive configuration workflow. It establishes the issue tracker, triage labels, and glossary/ADR locations under `docs/agents/`. Installation does not choose those preferences or create tickets.

Keep the existing project conventions during setup. Maintain `DESIGN.md` as shared, version-controlled documentation, and preserve the English-only requirement when editing agent instructions.

## Updates and inspection

Run updates from the project root:

```powershell
npx skills@latest update --project --yes
```

Inspect the installed agents and skills:

```powershell
npx skills@latest list --agent codex claude-code
```

Re-run `add` with the released skill selection when adopting newly released skills. Review changes to the copied files and lockfile. Keep agent-specific copies aligned when making local edits.

## Persistent project language

`AGENTS.md` is the authoritative project instruction file. It requires English for all documentation, source code and identifiers, comments, UI text, examples, and generated materials.

`CLAUDE.md` imports `AGENTS.md` so Claude Code uses the same instructions, including versions that do not load `AGENTS.md` directly. The rule remains outside third-party skill directories and survives package updates.

`DESIGN.md` is shared, version-controlled documentation. Skill setup and updates must preserve this convention and the English-only requirement.

## License

The upstream suite is MIT licensed. Its copyright and license notice are preserved in [mattpocock-skills-LICENSE.txt](third-party/mattpocock-skills-LICENSE.txt).
