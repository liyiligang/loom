# Workspace design

The first workspace follows Times' session, conversation, and execution flow. It is an operational tool for a developer controlling agents on several machines.

## Tokens

| Token | Value | Role |
| --- | --- | --- |
| Harbor | `#142c43` | Persistent session rail |
| Paper | `#f4f7fa` | Workspace background |
| Ink | `#17324d` | Primary text |
| Blue | `#316bdd` | Actions and selected state |
| Slate | `#66798c` | Secondary text |
| Pine | `#2b8063` | Connected status |

Segoe UI provides a clear humanist interface on Windows, with system sans-serif fallbacks elsewhere. A system monospace stack is reserved for code, command output, and working directories.

## Layout

```text
+------------------+-----------------------------------------+
| Loom             | Session title           Connection state|
| New session      +-----------------------------------------+
|                  | Conversation and expandable execution   |
| Session list     | details, with inline approvals           |
|                  |                                         |
|                  +-----------------------------------------+
| Agent connections| Task input and interruption             |
+------------------+-----------------------------------------+
```

Content is left aligned. The conversation has a readable maximum width; execution details can scroll horizontally. On small screens, a button opens the session rail and the workspace occupies the full width.

## Review against the brief

The main surface is a conversation rather than a dashboard of statistics. Connection management stays close to session selection. The small woven logo is the one expressive element; the remaining interface uses quiet, functional controls. Empty states guide users to connect a service and choose a project. Status comes from real server state. Keyboard focus, labeled inputs, narrow layouts, and reduced-motion settings are part of the implementation.
