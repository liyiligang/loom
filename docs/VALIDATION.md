# First slice validation

Date: 2026-11-04. Environment: Windows, Go 1.26.1, Node.js 24.10.0, Codex CLI 0.125.0.

## Completed checks

- Frontend TypeScript checking and production compilation.
- Go compilation, vet, and package checks after building the embedded frontend.
- Linux cross-compilation with CGO disabled; Linux runtime behavior remains unverified.
- Startup creates SQLite and serves the production page from one executable.
- A conflicting port returns a clear startup error.
- Browser API requests from an untrusted origin return HTTP 403; mutation requests with a non-JSON content type return HTTP 415.
- An actual local Codex app-server completes `initialize` / `initialized`, reports a compatible response, and creates an empty session in the configured target directory.
- Restarting the controller retains endpoint settings and session metadata, reconnects the actual service, and reports the empty-session rollout limitation without replacing native history.
- A newly created empty session successfully synchronizes on its original live connection.
- Protocol inspection confirmed `thread/read` works for empty live metadata and rejects `includeTurns` before the first user message. `thread/resume` rejects a thread without a rollout. These cases informed synchronization behavior.
- The browser automatically discovers the actual local Codex service without starting a task.
- Chromium checks covered the desktop workspace, connection dialog, session navigation, a 390-pixel mobile viewport, and browser draft persistence. No JavaScript page errors or horizontal viewport overflow were observed.

The verification service used a separate temporary data directory. No model task was submitted. Startup and handshake checks do not establish model authentication, quotas, tool permissions, or task-level compatibility.

## Outstanding validation

Automated behavioral test boundaries were proposed for the public HTTP/WebSocket API and a simulated Codex service. These tests have not been added; current Go package checks do not contain behavioral test cases.

Before treating the build as a validated first release, cover:

- Real text tasks, live output order, tool calls, file changes, and interruption confirmation.
- Native approval resolution, questions, duplicate replies, stale generations, and cancellation.
- Acknowledgement loss, delayed notifications, concurrent snapshot refreshes, reconnection during streaming, and terminal-state recovery.
- Restart history, multiple browser tabs, multiple sessions, and submission deduplication under failure.
- Authenticated LAN connections and discovery across actual machines.
- Claude SDK behavior, Loom nodes, mDNS, Linux runtime checks, and the remaining M4 capabilities.

## Reproduce build checks

```sh
npm --prefix web ci
npm --prefix web run build
go test ./...
go vet ./...
go build ./cmd/loom
```

The frontend build must run first because Go embeds the generated asset directory. Build checks are distinct from the outstanding task-level tests above.
