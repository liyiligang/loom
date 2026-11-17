# Agent skills

Loom uses the released suite from [AI Hero](https://www.aihero.dev/skills), maintained in [mattpocock/skills](https://github.com/mattpocock/skills).

## Installed scope

- 27 released skills, selected from the upstream plugin manifest.
- Project-local installation for Codex and Claude Code.
- Canonical Codex files in `.agents/skills/` and a generated Claude Code mirror in `.claude/skills/`.
- Copies rather than symbolic links for Windows compatibility.
- Upstream installation provenance recorded in root `skills-lock.json`; current local mirror hashes recorded separately in `scripts/skills-lock.json`.
- No global skill or plugin installation.

The upstream repository also contains draft and miscellaneous skills. They are outside the released suite and are not installed here. Do not use `--skill '*'` when the intent is to install only the released set.

Initial installation was checked against upstream commit `4ae2317ddb22b046e26455e4f9c9b3aaaf62212d`, whose plugin manifest reports version `1.3.2`. The installer version used was `skills@1.7.6`. The project now maintains local adaptations; the upstream lockfile is provenance rather than proof that the adapted files still match upstream.

## Install the upstream suite in another project

Loom checkouts already contain the adapted suite. To install the upstream suite in a different project, run the following PowerShell commands from that project's root. Node.js and npm are needed for the installer; the installed skills are files and do not require a background service.

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

Loom's shared tracker, domain, and workflow configuration is already in `docs/agents/` and referenced by `AGENTS.md`. Engineering work can start directly. To change that configuration, explicitly invoke `setup-matt-pocock-skills`:

- Codex: select the skill or invoke `$setup-matt-pocock-skills`.
- Claude Code: invoke `/setup-matt-pocock-skills`.

The setup skill reuses existing choices and asks only about consequential missing preferences. It is not a prerequisite for each task. Standalone specs and tickets use local Markdown by default; Loom-managed work reuses its supplied material source and stage protocol.

Keep the existing project conventions during setup. Maintain `DESIGN.md` as shared, version-controlled documentation, and preserve the English-only requirement when editing agent instructions.

## Updates and inspection

Edit `.agents/skills/` as the single source of truth, including references, scripts, assets, and native invocation policies. Then run:

```powershell
node scripts/skills.mjs sync
node scripts/skills.mjs check
```

Inspect the installed agents and skills:

```powershell
npx skills@latest list --agent codex claude-code
```

Sync copies complete skill resources, preserves unowned mirror files, and rejects independent mirror changes before writing. Reconcile such edits into the canonical source before retrying. The project check scripts and CI also check mirror consistency.

Evaluate upstream updates in an isolated checkout, compare them with the locally adapted source, and apply the useful changes to `.agents/skills/` before syncing. Running the upstream installer or updater directly over Loom's installation can overwrite its workflow adaptations. Preserve credits and existing explicit-only invocation settings when adopting updates or newly released skills.

## Persistent project language

`AGENTS.md` is the authoritative project instruction file. It requires English for documentation, source code and identifiers, comments, examples, and generated materials. Interface copy supports English and Simplified Chinese through English translation keys in `web/src/i18n/`.

`CLAUDE.md` imports `AGENTS.md` so Claude Code uses the same instructions, including versions that do not load `AGENTS.md` directly. The rule remains outside third-party skill directories and survives package updates.

`DESIGN.md` is shared, version-controlled documentation. Skill setup and updates must preserve this convention and the English-only requirement.

## License

The upstream suite is MIT licensed. Its copyright and license notice are preserved in [mattpocock-skills-LICENSE.txt](third-party/mattpocock-skills-LICENSE.txt).
