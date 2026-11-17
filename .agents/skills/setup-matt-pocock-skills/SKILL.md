---
name: setup-matt-pocock-skills
description: "Configure or update shared tracker, domain, and workflow pointers for both Codex and Claude."
disable-model-invocation: true
---

# Configure the engineering workflow

Read existing `AGENTS.md`, `CLAUDE.md`, `docs/agents/`, glossary, and relevant design before changing configuration. This repository already has shared configuration; update it only when the task calls for a change. Ordinary implementation and review do not require setup.

## Resolve only missing choices

Preserve the selected tracker and existing label vocabulary. When none is configured, local Markdown is the reversible default; a Git remote alone does not authorize publishing issues there. Ask when the user needs an external tracker or when existing configuration conflicts. Record remote workflow and authorization requirements without changing Git remotes or contacting a service merely to finish local setup.

Use the existing glossary/domain layout. Missing ADR files are created lazily for actual consequential decisions. Define label mappings only when triage uses them.

## Shared configuration

Keep shared conventions and navigation pointers in `AGENTS.md`. When `CLAUDE.md` imports `AGENTS.md`, preserve that import and avoid duplicating shared configuration there. If both contain shared rules, reconcile them into `AGENTS.md` while preserving harness-specific Claude instructions.

Use `docs/agents/issue-tracker.md`, `domain.md`, `triage-labels.md`, and `workflow.md` for details. Reuse existing files and the project's material paths. In a Loom task, preserve preparation and workspace ownership instead of installing a competing lifecycle.

For a different repository, the existing [local tracker](issue-tracker-local.md), [GitHub](issue-tracker-github.md), [GitLab](issue-tracker-gitlab.md), [domain](domain.md), and [label](triage-labels.md) templates are starting references. Adapt them to actual conventions and write English project materials. External label creation or publication uses only the user's selected and authorized workflow.

## Complete

Verify that both harnesses reach the same shared configuration and that relevant pointers resolve. For this repository, edit skills in `.agents/skills`, synchronize with `node scripts/skills.mjs sync`, and validate with `node scripts/skills.mjs check`. Report the configured workflow and any external capability that remains unavailable; local readiness does not establish remote authentication.
