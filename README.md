# Loom

Loom is a local agent controller that provides a unified web interface for Codex and Claude Code.

Starting Loom starts its web server and detects installed Codex and Claude Code CLIs. On the first console visit, confirm the detected agents once. Loom then manages their local processes and reconnects them on later starts. Create a session in the default project directory or choose a folder, send tasks, and inspect their execution.

The development build provides local Codex over stdio, a local Claude Agent SDK bridge, remote Codex over WebSocket, a bilingual Vue workspace, and SQLite history. Remote Loom nodes and mDNS remain planned.

Requirement documents, decisions, prototypes, test cases, datasets, Loop Design, and design documents are stored as files beneath `docs/requirements/<requirement ID>/` in the requirement's selected document-source worktree or project directory. Ask AI to create materials; their tabs support viewing, editing, downloading, and linking saved files. Documents use Markdown, test cases use YAML/JSON, and datasets use CSV/JSONL/JSON or externally added Parquet. SQLite stores file metadata and relationships, while the files retain their bodies and can be versioned with Git. Requirement AI messages receive the selected material source's file catalog and read relevant files through native tools. Back up both project/worktree files and the Loom data directory.

## Build and run

Build requirements: Go 1.26.1+, Node.js 22.12+ or 24+, and npm. The scripts build and embed both the frontend and the pinned Claude bridge. At runtime, local Codex needs the installed Codex CLI; local Claude additionally needs Node.js 22+ and the installed Claude Code CLI. The compiled controller serves the console and connects to Codex without Node.js. Use the existing CLI login and configuration; Loom does not install agents or request model credentials in the console.

On Windows, from the project root:

```powershell
./scripts/build.ps1
./bin/loom.exe
```

On macOS or Linux:

```sh
sh scripts/build.sh
./bin/loom
```

Open [http://127.0.0.1:7331](http://127.0.0.1:7331). Loom prints this address when HTTP serving starts. A port conflict stops startup with a clear error.

Build both asset directories before compiling or checking Go: `web/embed.go` embeds `web/dist`, and `bridge/claude/embed.go` embeds `bridge/claude/dist`. A fresh checkout should run the platform build script first. npm/npx distribution is planned; this repository does not yet publish an installable launcher.

For development, run the build script once, then run these commands in separate terminals:

```sh
go run ./cmd/loom
npm --prefix web run dev
```

Open [http://127.0.0.1:5173](http://127.0.0.1:5173) for Vite hot reload. Vite proxies `/api` to the controller on port 7331. Restart Go after backend changes.

## Development checks and skills

Run `./scripts/check.ps1` on Windows or `sh scripts/check.sh` on macOS/Linux. The command checks skill definitions and mirror consistency, runs tooling/frontend/bridge tests, builds embedded assets, and runs Go formatting, vet, and tests. Missing or stale direct dependencies are installed automatically; use `-InstallDependencies` or `--install` to force a clean install after other lockfile changes. GitHub Actions runs these same checks on Windows and Linux.

Edit project skills in `.agents/skills/`, then run `node scripts/skills.mjs sync`. `.claude/skills/` is the generated mirror, including supporting references, scripts, and assets. Sync detects independent mirror changes before writing. `node scripts/skills.mjs check` validates metadata, invocation settings, reference links, and drift. Shared task sizing, tracker defaults, Loom ownership, and validation guidance live in [docs/agents/workflow.md](docs/agents/workflow.md); both harnesses reach them through `AGENTS.md`.

## Local setup

1. Install and sign in to Codex or Claude Code with its normal CLI workflow.
2. Start Loom and open the printed console address.
3. Review detected paths and versions, then click **Confirm and enable**. No connection fields are required.
4. Click **Add project** to register a project, then add requirements with its **+** button. The project defaults to Loom's startup directory; **Choose folder** changes it without typing a path. Select a requirement and send a message in its **AI conversation** to align documents and design. Its conversation is created automatically on the first message. Use the + button on a requirement row to create a feature session manually, or open an existing one from the sidebar.

Detection checks PATH, common package-manager locations, and known macOS application locations using bounded `--version` probes. It runs on startup and through **Detect local agents**. Missing agents show installation guidance and can be detected again. Confirmation is stored in the selected data directory and applies to installed supported agents found on later scans. Explicitly disconnected agents stay disconnected when rescanned.

After confirmation, Loom owns `codex app-server --listen stdio://` and, when available, the Claude SDK bridge. No task runs before the user submits one. Existing CLI settings and login remain on this computer. The bridge uses the detected Claude executable rather than its bundled SDK CLI. Protocol readiness does not prove model authentication or quota availability; sign-in errors require the agent's normal terminal login workflow.

## Remote Codex connections

Choose **Connect to a remote service instead** during setup, or **Connections → Add remote service** later. Saved remote services skip local setup on subsequent visits; **Detect local agents** explicitly opens it when local consent is missing. Remote connections retain manual name, WebSocket URL, optional service token, and target-directory fields. A remote service must already be running. For example, on a Linux target with an existing private token file:

```sh
codex app-server --listen ws://0.0.0.0:3000 --ws-auth capability-token --ws-token-file /absolute/path/to/codex-token
```

Connect with the actual target address, such as `ws://192.168.1.20:3000/`. The working directory and model login belong to that machine. **Discover again** explicitly starts bounded LAN service probing; opening the console no longer starts a subnet scan.

Codex sessions use workspace-write sandboxing and on-request approvals. Claude uses the SDK's default permission mode and existing user/project/local settings; existing allow rules may approve tools before the callback. Loom provides approvals, questions, and interruption with agent confirmation. Closing the browser leaves the backend connections running; stopping Loom stops its owned agent processes.

## Current build

- Go startup, SQLite migrations, and an embedded Vue 3 frontend.
- Startup CLI detection, persisted first-run confirmation, managed local agent connections, and folder selection.
- Saved remote Codex WebSocket endpoints, separate service credentials, and automatic reconnection.
- Session creation, renaming, task submission, streamed messages, command/tool/file detail displays, and interruption.
- Approval and question handling with connection generation checks and native resolution confirmation.
- Persisted task intent, request ID deduplication, event cursors, incremental session snapshots, and browser drafts.
- Output batching with immediate lifecycle and approval updates, plus simulated-protocol regression tests.
- Explicit IPv4 discovery on configured ports, with progress, manual entry, a 512-host limit, 32 workers, and a 20-second budget. Services requiring authentication remain unverified until a credentialed handshake succeeds.

The streaming and interaction paths have simulated-protocol coverage and still need real task-level integration validation. Checks completed so far are documented in [docs/VALIDATION.md](docs/VALIDATION.md).

Known limits:

- Local Claude is implemented with SDK 0.3.318 and simulated behavioral coverage; real Claude task compatibility still needs validation. Remote Claude, target nodes, and mDNS are not implemented. IPv6 works through manual addresses; discovery does not enumerate IPv6.
- Codex cannot restore an empty thread after its connection closes because no rollout exists before the first task. Empty sessions can be synchronized on their original live connection.
- Recovery assumes a single Loom writer per session. A lost acknowledgement is reconciled only against an identifiable native turn; ambiguous outcomes keep the session blocked rather than resubmitting the task.
- Transient execution output may be unavailable after upstream reconnection. Local recorded history is retained. Individual text/output entries have a 1 MiB limit; history pagination remains future work.
- The browser server accepts loopback binds only. LAN browser access needs a later authentication implementation.

## Runtime options

| Option | Default | Purpose |
| --- | --- | --- |
| `--bind` | `127.0.0.1` | Loopback browser listener |
| `--port` | `7331` | Browser port |
| `--data` | `data` | SQLite and service credential directory |
| `--probe-ports` | `3000` | Up to eight comma-separated discovery ports |

The database is `<data>/loom.db`. Remote agent connection tokens remain in separate credential files: Windows uses the current user's DPAPI protection; other platforms use owner-only files. Third-party credential text uses AES-256-GCM encryption in SQLite, with a separate master key at `<data>/credentials/vault.key`. Back up the database and this key together. Windows protects the master key with DPAPI, so restoration requires the same Windows user and machine. The default data directory and build output are excluded from Git. Real credentials must remain outside version control.

Open **Credentials** with the key icon at the bottom left of the sidebar to add, edit, view, copy, or delete third-party credentials. Names are unique; values are masked by default. Leave an existing value blank while editing to retain it. Select a project, switch to the **Bind credentials** tab in its main-area page, activate credentials, then save. Local Codex and Claude sessions in that project share the activation list; other projects start with no grants. Settings survive restarts. Deactivation, deletion, rotation, task completion, or moving a session to another project cancels affected in-flight requests.

Loom supplies session-scoped `list_credentials` and `request` MCP tools to local agents. The tools use saved values internally for public HTTPS API requests: Use complete-text or named-field placeholders in headers/body fields; plain tokens support Bearer authentication, and recognized AWS key fields support SigV4 signing. Credential values are not added to prompts or a shared agent environment. Responses mask the selected credential's values and common encodings. Redirects and private-network targets are refused. Agents keep their normal tool permission policies. Other platform-specific signature algorithms need an additional adapter; remote credential tool delivery is not implemented.

## Design

See [DESIGN.md](DESIGN.md) for the architecture, agent connections, discovery, data model, API, and implementation milestones.

Requirements provide nine numbered tabs: AI conversation, Requirement documents, Design documents, Prototypes, Grill me, Loop Design, Branch group, Test cases, and Test datasets. The conversation is the initial full-width panel and supports document/design alignment. Ask AI to organize and create the materials; their category panels have no Add file button. The first message starts the discussion automatically, and switching tabs preserves its history and draft. Opening conversation retains the last selected content section as context for the next message. Discussions use the project directory and receive the group's membership plus its independently selected document-source catalog. Bind a prefix such as `feat/console/` in the Branch group tab after Loop Design, with `main` prefilled as the base reference, without creating a Git branch. Add member branches as needed; each receives a fixed worktree. New sessions explicitly choose a ready group member, while existing sessions keep their own directories and history. The seven material tabs manage actual project files and relationships. Prototypes render self-contained HTML demos with inline CSS/JavaScript and sample data in an isolated interactive preview; ask AI to create them, then select or refresh the saved file. Selecting an execution branch does not change the shared document source. AI orchestration of implementation sessions remains planned.

Use the permissions button in a session or requirement AI conversation to choose Workspace access or Full access. Workspace access is the default. Full access allows agent-machine file/network access and skips routine native tool approvals; managed runtime policies may still apply. The choice persists and applies to the next task, while active work and existing approvals keep their current state. Requirement preferences are available before the first message, and feature sessions keep independent choices.

Use **Settings → AI models** to maintain a shared list of model names and identifiers for Codex and Claude Code. The list starts empty and is saved in SQLite. Projects choose defaults from this list in their **AI models** tab; requirements can inherit or override them through the model button in AI conversation. Scope changes apply to the next conversation turn or preparation stage, preserving active tasks and history. Editing the list does not rewrite existing selections.

Start the preparation workflow by entering a brief in the requirement conversation and clicking **Start requirement preparation**. AI writes the requirement and design documents, records Grill me questions, then aligns the files using your answers. Review and confirm the actual documents to generate a real visual Loop preview. Source changes block stale confirmation and planning; accepted source and plan snapshots stay in project files, while SQLite stores progression and relationships. Preparation recovers across restart through the existing native turn ledger. Automatic development/test/repair execution remains a later increment.

The project's Skills tab supports adding, editing, and deleting project-installed definitions. Add accepts a Unicode Skill display name, stores it in the definition metadata, and automatically creates an English starter `SKILL.md` in `.agents/skills/<identifier>/`; edit opens the full source including front matter. Delete asks for confirmation before removing the selected installation and its supporting files. Linked source files remain intact. Current-directory AI activity and stale versions block file mutations.

Double-click project, requirement, or session names in the sidebar to rename them; Enter or blur saves and Escape cancels. Group members can develop concurrently in separate worktrees, while one active writer is allowed in each local directory. Saved group membership, preparation intent, document sources, and session bindings survive restart. Failed member preparation can be retried in the Branch group tab. Git must be available on PATH. Remote worktrees are prepared manually; automatic integration and archival remain future work. See sections 16 and 20 of the design for ownership and recovery details.

## Purpose

Loom provides a central interface for working with Codex and Claude Code on the local machine or other machines on the LAN:

- View available machines and agents, including their connection status.
- Discuss a requirement with an agent, create a feature session with its + button, or open an existing session.
- Send tasks and watch responses, tool calls, command output, and file changes in real time.
- Respond to approval requests and questions directly in the browser.
- Interrupt running tasks, review history, and continue working.

The first release focuses on a useful UI for everyday agent operations. Automated scheduling and orchestration across multiple agents are planned for later releases.

## Workflow

1. Prepare a signed-in local Codex or Claude Code installation, or an authenticated remote Codex app-server.
2. Start Loom and open the printed browser URL, normally `http://127.0.0.1:7331`.
3. Confirm detected local agents once, or explicitly add a remote service with its address and optional token. Saved services reconnect; **Discover again** starts bounded LAN probing when requested.
4. Add a project and requirement. For an understood change, use its conversation or a feature session directly. For document preparation, enter a brief and click **Start requirement preparation**.
5. Preparation writes requirement/design drafts, records Grill questions, and aligns documents using the submitted answers. Review and confirm the actual files to generate a Loop preview. This confirmation does not start development.
6. Bind a branch group and prepare members when execution needs isolated workspaces. A new feature session selects a ready member; its working directory stays fixed while the shared material source remains independently selected.
7. Send an implementation task, watch live output, answer approvals or questions, and review changes and checks. Automatic development Loop execution and integration remain planned.

Local agents and direct remote Codex are implemented. Remote Claude and node management remain planned, and real task-level validation is still required.

## Technology

| Component | Technology | Responsibility |
| --- | --- | --- |
| Backend | Go | Web serving, discovery, agent connections, session management, and event delivery |
| Frontend | Vue 3 + TypeScript + Vite | Workspace, live conversation, execution details, and connection settings |
| UI reference | Times Vue 3 implementation and Element Plus | Layout, component organization, and conversation interactions |
| Database | SQLite 3 | Connection settings, session indexes, messages, execution events, and approval state |
| Live communication | WebSocket | Connection status, message deltas, and execution events |

The Go application serves the built frontend. Normal use starts Loom without a separate frontend server or database service.

## Agent connections

### Codex

Loom launches local Codex app-server over stdio and connects to remote app-server over WebSocket, using the same RPC session and turn operations. The Loom backend owns connections and credentials; the browser uses Loom's unified API.

Codex can be reached through an existing app-server or through a Loom node that manages the local process on the target machine.

### Claude Code

Claude Code uses a dedicated local adapter and a small embedded Claude Agent SDK bridge. Go manages the Node.js process over stdio; the bridge converts output, approval requests, questions, and interruption into unified operations. A network endpoint for remote Claude is future node work.

The implemented Claude bridge uses the TypeScript SDK, so machines using this integration also need Node.js. The web backend and node service use Go. Direct native Codex connections do not require the bridge.

### LAN discovery

The current Go backend reconnects saved addresses and performs bounded probes of configured ports. Manual address entry is available. Node mDNS advertisements are planned.

**Discovery targets agent services that expose a network endpoint.** Installing a CLI or opening a terminal or desktop client does not automatically expose a LAN service. Initial setup requires starting app-server or a Loom node on the target machine and configuring connection credentials.

## First release

- Automatically start the local web server.
- Discover LAN services, add addresses manually, display connection status, and reconnect.
- Provide a unified session interface for Codex and Claude Code.
- Create sessions, restore history, send tasks, and stream output.
- Display command and tool execution details and available file changes.
- Handle supported approval requests and user questions.
- Interrupt tasks and persist local history and settings.

Adapters report each agent's capabilities. The UI exposes only operations supported by the connected service, such as steering a running turn, listing models, or displaying file change details.

## Development sequence

1. Go web server, Vue 3 page, SQLite migrations, and frontend packaging.
2. Codex app-server integration for session creation, task submission, streaming responses, and interruption.
3. Session history, execution details, approval interactions, and connection recovery.
4. Loom nodes, LAN discovery, and the Claude adapter to complete the interface for both agents.

The foundation, local adapters, and direct remote Codex implementation are present. Complete real task-level validation and recovery coverage before adding remote Loom nodes. `DESIGN.md` is included in version control.

## Agent skills

The 27 released skills from [Matt Pocock's Skills for Real Engineers](https://www.aihero.dev/skills) are installed locally for Codex and Claude Code. See [the installation and usage guide](docs/SKILLS.md) for setup, updates, and the complete installation command.

## Project language

Documentation, code identifiers, comments, examples, and generated project materials use English. The interface supports English and Simplified Chinese. On first use it selects the first supported language in the browser's language preferences, which normally follow the operating system settings; unsupported preferences fall back to English. The sidebar footer language menu offers only English and Simplified Chinese and shows the active language. A saved manual choice takes precedence and is shared across tabs in the same browser.

The sidebar footer sun/moon button switches between light and dark themes. Before a manual choice, appearance follows the system color scheme. Your choice is saved in the browser, restored on refresh, and synchronized across tabs. First-run confirmation includes the same switch.

Interface messages, accessibility labels, settings pages, status labels, timestamps, and controller-owned errors follow the selected language. User names and text, agent replies, paths, commands, and raw execution data retain their original content.

### Credential text and settings

Open **Credentials** from the main-area header, paste credential text, and save. A name is optional; there is no type selector. Multiline text, JSON, environment assignments, and other UTF-8 formats are preserved exactly (up to 64 KiB). Older Token and AK/SK records remain readable. Activate saved credentials in the project’s **Bind credentials** tab before local AI sessions can use them.

Credential management and other settings use the main workspace with a back control. Folder browsing and deletion confirmation stay inline. The request tool supports `{{content}}` and named JSON/environment/flat YAML/header fields without returning their values to the AI. AWS SigV4 is supported; other signing protocols still need an adapter.
