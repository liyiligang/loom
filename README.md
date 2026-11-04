# Loom

Loom is a local agent controller that provides a unified web interface for Codex and Claude Code.

Starting Loom starts its web server. Users open a local browser address to discover accessible agent services on the LAN, connect to a target machine, create a session in a project directory, send tasks, inspect execution, and continue the conversation.

The first development build is runnable. It provides direct Codex connectivity, a Vue workspace, SQLite history, and bounded service discovery. Claude Code, Loom nodes, and mDNS are the next milestone.

## Build and run

Requirements: Go 1.26.1+, Node.js 22.12+ or 24+, and npm. The Codex protocol was checked against CLI 0.125.0. Node.js is needed to build the frontend; the compiled controller serves its embedded assets without Node.js.

On Windows, from the project root:

```powershell
./scripts/build.ps1
./bin/loom.exe
```

On Linux:

```sh
sh scripts/build.sh
./bin/loom
```

Open [http://127.0.0.1:7331](http://127.0.0.1:7331). Loom prints this address when HTTP serving starts. A port conflict stops startup with a clear error.

Frontend assets must be built before compiling or checking the Go application because `web/embed.go` embeds `web/dist`.

For development, build the frontend once, then run these commands in separate terminals:

```sh
go run ./cmd/loom
npm --prefix web run dev
```

Open [http://127.0.0.1:5173](http://127.0.0.1:5173) for Vite hot reload. Vite proxies `/api` to the controller on port 7331. Restart Go after backend changes.

## Connect Codex

Start an authenticated Codex app-server on the target machine. For local use:

```sh
codex app-server --listen ws://127.0.0.1:3000
```

In Loom, select **Connections**, enter a service name and `ws://127.0.0.1:3000/`, and save. Once connected, select **New session**, enter a project directory on the target machine, and describe a task.

For a LAN service, use a non-loopback listener with capability-token authentication. For example, on a Linux target with an existing private token file:

```sh
codex app-server --listen ws://0.0.0.0:3000 --ws-auth capability-token --ws-token-file /absolute/path/to/codex-token
```

Connect using the target machine's actual address, such as `ws://192.168.1.20:3000/`, and enter that service token in Loom. The working directory belongs to the target machine. Model login and provider settings stay with Codex.

Sessions use workspace-write sandboxing and on-request approvals. Loom provides **Allow once**, **Decline**, user question forms, and **Interrupt**. Interrupt requests wait for agent state confirmation. Closing the browser leaves the backend's agent connections running.

## Current build

- Go startup, SQLite migrations, and an embedded Vue 3 frontend.
- Saved Codex WebSocket endpoints, separate service credentials, and automatic reconnection.
- Session creation, renaming, task submission, streamed messages, command/tool/file detail displays, and interruption.
- Approval and question handling with connection generation checks and native resolution confirmation.
- Persisted task intent, request ID deduplication, event cursors, session snapshots, and browser drafts.
- Automatic IPv4 discovery on configured ports, with progress, manual entry, a 512-host limit, 32 workers, and a 20-second budget. Services requiring authentication remain unverified until a credentialed handshake succeeds.

The streaming and interaction paths are implemented but still need task-level integration validation. Checks completed so far are documented in [docs/VALIDATION.md](docs/VALIDATION.md).

Known limits:

- Claude Code, target nodes, and mDNS are not implemented yet. IPv6 works through manual addresses; discovery does not enumerate IPv6.
- Codex cannot restore an empty thread after its connection closes because no rollout exists before the first task. Empty sessions can be synchronized on their original live connection.
- Recovery assumes a single Loom writer per session. A lost acknowledgement is reconciled only against an identifiable native turn; ambiguous outcomes keep the session blocked rather than resubmitting the task.
- Transient execution output may be unavailable after upstream reconnection. Local recorded history is retained. Individual text/output entries have a 1 MiB limit; history pagination and delta batching remain future work.
- The browser server accepts loopback binds only. LAN browser access needs a later authentication implementation.

## Runtime options

| Option | Default | Purpose |
| --- | --- | --- |
| `--bind` | `127.0.0.1` | Loopback browser listener |
| `--port` | `7331` | Browser port |
| `--data` | `data` | SQLite and service credential directory |
| `--probe-ports` | `3000` | Up to eight comma-separated discovery ports |

The database is `<data>/loom.db`. Credentials are separate from SQLite: Windows uses the current user's DPAPI protection; other platforms use owner-only files. The default data directory and build output are excluded from Git. Real credentials must remain outside version control.

## Design

See [DESIGN.md](DESIGN.md) for the architecture, agent connections, discovery, data model, API, and implementation milestones.

## Purpose

Loom provides a central interface for working with Codex and Claude Code on the local machine or other machines on the LAN:

- View available machines and agents, including their connection status.
- Select an agent and project working directory, then create or restore a session.
- Send tasks and watch responses, tool calls, command output, and file changes in real time.
- Respond to approval requests and questions directly in the browser.
- Interrupt running tasks, review history, and continue working.

The first release focuses on a useful UI for everyday agent operations. Automated scheduling and orchestration across multiple agents are planned for later releases.

## Workflow

1. Prepare an authenticated Codex or Claude Code installation on the target machine and start a service that Loom can connect to.
2. Start Loom. The planned default web address is `127.0.0.1:7331`, and the terminal prints the browser URL.
3. Open `http://127.0.0.1:7331`. The page automatically starts LAN discovery and displays the results.
4. Configure credentials when connecting to a service for the first time. Previously configured services can reconnect automatically. Manual address entry is also available.
5. Select an agent and a project directory on the target machine, create a session, and send a task.
6. Watch live output, respond to approvals or questions, and continue the conversation after the task finishes.

This workflow describes the full first-release target. The current build supports its direct Codex path; the Claude and node paths remain planned.

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

Following the Times backend implementation, Loom connects to Codex app-server over WebSocket and uses its RPC protocol to operate sessions and turns. The Loom backend owns connections and credentials; the browser uses Loom's unified API.

Codex can be reached through an existing app-server or through a Loom node that manages the local process on the target machine.

### Claude Code

Claude Code uses a dedicated adapter. A Go-based Loom node runs on the target machine, provides the network endpoint, and connects to Claude Code through a small Claude Agent SDK bridge. The bridge converts output, approval requests, and questions into unified events.

The planned Claude bridge uses the TypeScript SDK, so target machines using this integration also need Node.js. The web backend and node service use Go. Direct native Codex connections do not require the bridge.

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

The foundation and direct Codex implementation are present. Complete task-level validation and recovery coverage before adding Loom nodes and Claude. `DESIGN.md` is included in version control.

## Agent skills

The 27 released skills from [Matt Pocock's Skills for Real Engineers](https://www.aihero.dev/skills) are installed locally for Codex and Claude Code. See [the installation and usage guide](docs/SKILLS.md) for setup, updates, and the complete installation command.

## Project language

All project documentation, code, comments, UI text, examples, and generated materials use English.
