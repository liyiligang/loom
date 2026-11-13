# Workspace design

The first workspace follows Times' session, conversation, and execution flow. It is an operational tool for a developer controlling agents on several machines.

## Tokens

| Token | Light | Dark | Role |
| --- | --- | --- | --- |
| Surface | `#ffffff` | `#25282e` | Conversation background |
| Sidebar | `#f9f9f9` | `#2e3239` | Persistent session rail |
| Raised surface | `#ffffff` | `#323740` | Composer, fields, and settings |
| Text | `#202123` | `#e4e8ef` | Primary text |
| Muted | `#676767` | `#abb5c4` | Secondary text |
| Accent | `#202123` | `#dde3ec` | Primary actions |
| Border | `#e5e5e5` | `#414751` | Separators and quiet cards |
| Success | `#21785c` | `#80bea6` | Connected and completed status |

The native system font stack uses macOS system fonts and Segoe UI on Windows, with Chinese-capable fallbacks. A system monospace stack is reserved for code, command output, and working directories.

## Appearance

The light palette uses a white conversation and pale gray rail. The dark palette uses layered graphite surfaces, silver text, and restrained blue-gray accents. Its rail is lighter than the conversation, with a subtle top-to-bottom shading and a quiet separator. Raised composers and cards distinguish interaction from content without large near-black areas. Code, tools, interactions, errors, native controls, settings pages, and notifications share semantic tokens rather than independent color overrides.

A compact sun/moon button in the sidebar footer switches between the two themes. It is also available in first-run confirmation. Before a manual choice, appearance follows the system color scheme and responds to system changes. Manual selection is stored in the browser, restored before the application renders, and synchronized across tabs. Switching themes does not recreate the workspace, clear drafts, or reset question answers. The external startup script works with the existing Content Security Policy.

## Layout

```text
+------------------+-----------------------------------------+
| Loom             | Session title                           |
| Add project      +-----------------------------------------+
|                  | Conversation and expandable execution   |
| Session list     | details, with inline approvals           |
|                  |                                         |
|                  +-----------------------------------------+
| Agent connections| Task input and interruption             |
| Language / actions|                                         |
+------------------+-----------------------------------------+
```

The credential-management key icon, language selector, and theme control share a fixed action row at the bottom of the sidebar. The service list scrolls independently above it; selecting a service opens connection management. Project, requirement, and session names support double-click editing in the sidebar. On small screens, opening a main-area page closes the navigation drawer.

Content is left aligned. The conversation has a readable maximum width; execution details can scroll horizontally. On small screens, a button opens the session rail and the workspace occupies the full width.

## Languages

The sidebar footer language selector offers only English and Simplified Chinese on desktop and mobile, showing the active language even on first use. Initial selection uses browser language preferences; manual selection is saved locally and takes precedence. Legacy system preferences fall back to initial language detection. Changing language updates visible application copy and open pages without reloading or replacing drafts, questions, task content, or user-defined names. Element Plus uses the matching locale, and the document language, title, time display, and accessibility labels change together. Chinese-capable system fonts supplement Segoe UI.

## Review against the brief

The main surface is a conversation rather than a dashboard of statistics. Connection management stays close to session selection. The small woven logo is the one expressive element; the remaining interface uses quiet, functional controls. Empty states guide users to connect a service and choose a project. Status comes from real server state. Keyboard focus, labeled inputs, narrow layouts, and reduced-motion settings are part of the implementation.


## Local onboarding

The first visit opens a compact local-agent confirmation page showing names and availability. It includes its own language selector. Confirmation has one primary action and no connection fields. The secondary actions rescan installations or explicitly open remote configuration.

Managed local connection cards expose connection state and connect/disconnect actions. The remote form opens through **Add remote service** or editing a remote connection. Local session creation selects an installed agent and defaults the project directory; a directory browser changes it with clicks. Remote sessions retain target directory, optional model, and title fields. Local startup progress disables the welcome action until an agent is ready. A single available agent is selected automatically.


## Minimal interface copy

The homepage contains one task heading and a primary action. It omits product descriptions, capability lists, connected-service counts, and repeated navigation subtitles. Conversation empty states use one heading; the input uses a short task placeholder. Current execution state and the actual model remain visible, while keyboard instructions and successful connection notifications do not appear as persistent tips.

First-run confirmation uses one sentence, compact agent rows, one confirmation button, and small rescan/remote links. Executable paths, versions, and installation commands are omitted from the normal view. Installation help is available on demand when no agent is usable. Errors remain visible when they explain an unavailable agent.

Connection management omits the general introduction and managed-process description. Local detection lives here rather than in the global header. LAN discovery appears only in the remote workflow and shows results after an explicit request. Local sessions inherit the selected project directory and accept an optional feature name; agent selection appears when multiple compatible ready services exist. Repeated permission explanations are omitted from the session form.

Keep meaningful failures, unknown outcomes, prompting-stage approvals, questions, truncation notices, and current execution state. When a turn and its session contain the same error, display the explanation once. User input, agent responses, execution records, and approval actions retain their content and behavior.

## Conversation motion

Live assistant replies use a short progressive reveal between snapshots. Existing history displays immediately, while large recovery updates and corrections catch up without replay. Whole graphemes preserve Chinese text and joined emoji. Markdown blocks retain their DOM when their content is unchanged; selecting text pauses that reply's visible updates until the selection is released.

Tool rows stay compact. A spinner marks reported execution, a quiet check marks completion, and failed tools retain a visible diagnostic state. Details expand and collapse with a short height/opacity transition. The command and output stay readable; the full execution record is available through a secondary disclosure. Closed details are excluded from keyboard navigation and accessibility presentation.

New entries, interaction resolution, input focus, buttons, pages, and the mobile sidebar use subtle transitions. Loaded conversations do not replay entry animations. Motion is disabled by the system's reduced-motion preference, including progressive text buffering. The timeline follows visible output growth only while the reader is near the bottom and keeps their position when they browse earlier content.

## Project navigation

The sidebar always lists projects at the top level, requirements beneath each project, and feature sessions beneath each requirement. Selecting an item opens its content in the main area and preserves the tree and its expansion choices. Separate native chevron buttons expand each level independently; requirement names open their content page. Small chevrons rotate and panels animate their height; reduced-motion settings remove transitions. Project folders use compact badges, requirement labels use quieter typography, and feature sessions use single-line rows with a narrow selection marker on the guide. Agent names appear on session rows only when a requirement contains multiple agents; idle state remains accessible without repeating green dots on every row. Closed panels are excluded from keyboard navigation. Session navigation, membership changes, and streaming updates preserve manual collapses. Browser storage retains expansion choices.

**Add project** selects a local folder or a remote target directory, with an optional display name. Project rows contain navigation labels, separate disclosure chevrons, and one always-visible New requirement (+) button; its form opens in the main area without changing expansion choices. Requirement rows contain navigation labels, disclosure chevrons, and an always-visible New session (+) button matching the project action. Selecting a project shows peer Skills and Bind credentials tabs in the main area, with only the active panel visible; selecting a requirement shows its content tabs, including branch-group binding. Project/requirement creation forms return to the page that opened them. Manual session creation opens the new conversation; cancel returns to its requirement. Requirement names omit session counts. Labels retain their layout on hover and focus. Single-line service rows leave more space for navigation. Double-click a sidebar name or press F2 on the focused item to rename it inline. Enter or blur saves, Escape cancels, and failed saves keep the input with a localized error. Unchanged names preserve generated-name localization. Renaming leaves session membership and directories unchanged. The controller persists groups and membership. Existing sessions are assigned to a directory project under **General** without changing their conversation history.

The two themes retain their semantic colors, hierarchy guides, truncated long labels, and visible execution status. English and Simplified Chinese copy share typed translation keys. Local Git requirements can bind a branch group and prepare member worktrees as described below; legacy project/requirement grouping without Git remains metadata-only.

## Navigation width and indentation

The desktop sidebar is 320 px wide, reducing to 288 px on medium screens. Narrow screens use a drawer capped at 320 px or 90% of the viewport. Requirements use a 16 px inset and their session lists add only 8 px, keeping more horizontal space for titles. A narrow guide and selection marker preserve hierarchy without deep cumulative indentation.

## Requirement content

Material panels identify files by filename and omit internal storage paths and requirement-ID directories.

Select a requirement to open its content page without a back button. The tabs are numbered 1–9: AI conversation, Requirement documents, Design documents, Prototypes, Grill me, Loop Design, Branch group, Test cases, and Test datasets. AI conversation opens initially; the eight content tabs retain their relative development order. Only the selected panel is visible. The seven material tabs list files from the current worktree or project directory, with localized empty states directing users to organize content through AI conversation. These panels have no Add file button or manual creation form. Selecting a file shows its preview, download, edit action, and cross-category relationship selector. Save writes edits to an existing file, and Refresh discovers external changes. Text editing is limited to 1 MiB; larger files and Parquet can be downloaded. Unsaved edits survive category changes within the same page. Stale file/branch conflicts retain the draft, and Cancel discards it and loads current content. AI occupancy disables file writes. The Branch group tab contains prefix binding, member creation/retry, and independent document-source selection. Switching tabs leaves the sidebar unchanged, supports arrow keys and Home/End in the numbered order, and scrolls horizontally on narrow screens.
The Prototypes tab previews saved self-contained HTML demos after Design documents. It automatically opens an available file and lets users select other demo files, restart interactions, edit source, download and relate files. Inline styles/scripts and sample data work inside the isolated preview; external assets and real service calls are unavailable. Its empty state asks AI to generate the HTML. Existing document preparation and Loop stages are unchanged.

Ordinary session headers and requirement AI conversation headers include a permissions button showing the saved Workspace access or Full access choice. Its options explain normal approvals and unrestricted file/network access with routine approvals skipped. Changes persist and apply to the next task; an active task keeps its captured choice, and existing approvals still need a response. Requirement preferences can be selected before the first message without creating a native conversation. Feature sessions have independent choices. Sending and starting preparation are disabled while a permissions update is saving. Failures keep the selector open with a localized error.


The AI conversation tab uses the whole panel on desktop and narrow screens. Its timeline scrolls above the composer; other tabs display their content without a conversation column. Send a message to align documents, decisions, design, or tests; the first message automatically creates a conversation for that requirement. When multiple compatible agents are ready, choose one before sending. The conversation stays mounted, hidden, and inert while another tab is active. Opening it retains the last selected content section as the discussion context, initially Requirement documents, while preserving the same history, draft, streamed output, and approvals/questions. Requirement discussions do not add feature-session rows to the sidebar. They run in the project directory and retain their history when the group or its members change. Each message supplies group membership and the independently selected document-source directory, file paths, hashes, and relationships so AI can read the relevant files. Completion refreshes the file catalog while retaining unsaved browser edits. Interruption, synchronization, offline status, and local directory writer admission use the same controls and recovery rules as other conversations.

Requirement and session headers have no edit buttons; rename their sidebar items inline. Click the New session (+) button on a requirement row to open a creation form in the main area. Enter an optional title and select an agent on the same machine; the directory comes from the chosen ready group member or, for an unbound requirement, the project root. Successful creation opens the new feature conversation without expanding sidebar branches. Offline agents and unready worktrees disable creation, and errors preserve the form input. The requirement's AI discussion remains a separate persistent conversation. The Branch group tab manages the prefix and its member worktrees. A bound group requires choosing a ready execution branch in the new-session form. A group without members directs the user to add a branch first. AI orchestration of implementation sessions remains planned.

## Project Skills

Click a project folder or name to open its main-area page with Skills selected by default. Skills and Bind credentials are peer tabs; switching changes only the active panel and preserves catalog search and unsaved credential choices. The left sidebar keeps its project tree, and its chevron still expands requirements independently. The conversation directory opens the list for that session's actual directory, including worktrees. Each Skill shows its name and a short description; duplicate names show a source label, and hovering exposes installed paths. Lists above five entries have a search field. Refresh reads changes from disk, and selecting a Skill opens a read-only Markdown page in the main area. Back returns from the preview to the same catalog and search. The project page has no back button; use sidebar navigation to open another project or conversation. The sidebar structure and expansion choices stay unchanged throughout.

The view reads project-local `.agents/skills`, `.claude/skills`, `.codex/skills`, and `skills` folders. Symlink aliases to one definition are combined. Empty, unavailable, and partial results use concise localized states. Remote directories are unavailable in this local browser. Add skill opens an inline full-source editor with a Skill name and English starter. The required display name supports Chinese and other Unicode text up to 128 characters. The controller generates an ASCII installation identifier and keeps the standard directory/`SKILL.md` layout. The same name field appears during editing; changing it updates `metadata.display_name` while preserving native identity, instruction text, custom metadata, comments, and supporting files. Row edit actions load exact `SKILL.md` source, including front matter; saves preserve supporting files. A native modal dialog names the Skill and requests confirmation before deletion. Cancel and Escape dismiss it without deletion; the dialog traps focus, initially focuses Cancel, and keeps errors visible. Local directories/supporting files are removed only after confirmation; linked source files are preserved. External linked definitions can be copied into a local definition for editing, with a notice that supporting files are not copied. Shared root/category links that would affect other definitions are not mutable. No Skill is executed by these actions. Both themes retain the existing compact sidebar width; narrow layouts close the navigation drawer and show Skills in the main area.

## Requirement branch groups

Requirements bind a branch group prefix, such as `feat/console/`, in the Branch group tab after Loop Design. The base-reference field starts with `main` for a new group; existing groups show their saved base. Saving the group records a pinned base reference without creating or checking out a Git branch. Group prefixes must be available within the repository; validation errors preserve the form draft. The sidebar displays the group prefix and keeps its existing tree and expansion choices. Clicking the prefix opens the Branch group tab in the main area.

The member list shows each branch and preparation state. Add branch accepts a short name or an existing branch under the group prefix. Choose the pinned group base or a ready member as the starting point. New members prepare separate worktrees; existing matching worktrees can be adopted. Repeated creation preserves member identity, and failed preparation exposes Retry. Prefix editing retains existing member names, worktrees, and conversations.

Click the requirement New session (+) to choose a ready execution branch and compatible agent. Successful creation opens a conversation in that member's fixed directory. Multiple members require an explicit choice; a single member is preselected. An empty group disables creation and explains where to add a branch. Earlier conversations retain their native identities and directories when group members or the prefix change.

Requirement document source is a separate saved selector. New groups use the project directory; migrated groups retain their previous source. Selecting a group member changes the material panels and AI catalog to that worktree, while selecting Project directory restores the original source. This does not move or copy documents, change an existing session's execution branch, or discard branch-specific indices. Unsaved group, member, and source form drafts survive tab switching. Controls remain in the main area and fit narrow layouts.

Remote and non-Git directories retain their existing conversation workflow; automatic Git group management requires a local repository with a commit. A member's directory and branch remain visible in the resulting conversation context. One active writer is admitted per workspace; different workspaces can execute independently.

## Credential manager

The key icon at the left of the fixed sidebar footer opens **Credentials**, beside the language and theme controls. This is the single credential-management entry point. On narrow screens it is available in the navigation drawer, which closes when the page opens. The list sits beside an editor on desktop and above it on mobile. Paste any credential text into one multiline field and save; the name is optional. There is no type selector. Text is retained verbatim, with a 64 KiB UTF-8 limit.

Select a saved credential to rename it or paste replacement text. Leaving the text blank preserves the value. Viewing requires an explicit action, starts masked, and provides show/hide and copy controls. Deleting uses a short inline confirmation, with cancel beside the delete action. Changing selection or leaving the page clears plaintext from the component.

**Bind credentials** is a peer tab to **Skills** on the project page. Its panel contains a compact activation list with pill switches and the save action; project names are edited in the sidebar. Each switch changes the selection draft; Save applies it. Saving and remote scope disable the switches, and Space or Enter toggles a focused switch. The footer key icon opens the global credential page; **Back** returns to the same tab with its unsaved selections preserved. Saving the project makes the selected credentials available to its local AI sessions through scoped credential tools while keeping Bind credentials selected. The sidebar has no project settings, binding, rename, or session creation buttons. New requirement (+) remains the only action button on each project row.

Project/requirement/session creation, credential/Git binding, connections, local-agent setup, and Skill previews use the main area. Name editing stays inline in the sidebar. Folder selection expands within its form. Project and requirement content pages omit the back control. Detail and management pages retain it to return to their parent page without changing conversation drafts. These flows have no dialogs or overlay drawers; the mobile navigation drawer remains for navigation.

## Visual Loop Design prototype

The development-only Loop Design prototype embeds three layouts in the existing requirement page: `canvas`, `lanes`, and `focus`. Start it with `npm --prefix web run prototype:loop` while the local Loom service listens on 7331, then open `http://127.0.0.1:5173/?prototype=loop&variant=canvas`. The floating comparison bar switches layouts and updates the URL without replacing the sidebar. An optional `requirement` query selects an existing requirement.

The recommended canvas layout places document sources and the generation action above the graph, with node details on the right. Generation stops at preview. Selecting a node shows its task, source inputs, proposed branch, outputs, and pass conditions; preview task edits survive layout changes. A separate action starts the simulation. Normal wheel input scrolls the page; Ctrl/Command with the wheel zooms the graph. Drag pans, and Fit restores the complete graph. Selecting a node disables automatic selection following until the user enables it again.

The reference graph joins implementation with independent tests and review, then routes the evidence verdict to delivery, bounded repair, or missing-input attention. Simulated runs support pause, resume, single-step, pass/repair/missing-data scenarios, and a configurable attempt limit. A stopped run retains its failure events. Missing checks are blocked and require an explicit resume action. Status labels accompany violet active paths, teal completion, warm failure feedback, and amber attention. Effects can be disabled and respect reduced-motion preferences; animation pauses with the demonstration or a hidden tab.

The prototype notice explicitly identifies sample documents and simulated execution. No model call, branch creation, session mutation, or durable plan write occurs. State resets on reload. The existing requirement discussion stays mounted in the AI conversation tab, and other tabs keep their normal content. Production builds retain the current Loop Design material page. The real generation, plan-file, execution, and recovery contracts are specified in DESIGN section 21 and remain future implementation.

## Requirement preparation and generated Loop preview

The AI conversation composer supplies the brief for **Start requirement preparation**. A six-step progress strip links to the corresponding material tab. Native approvals/questions remain in the timeline. Grill me answers can be entered in the conversation or Grill me tab; answer drafts survive tab switches and reload. The alignment gate exposes the current Markdown bodies and requires explicit confirmation of unchanged source versions. Further revision requests return to alignment. Pause stops subsequent stages; normal interrupt handles an active native turn.

The production Loop Design panel renders the accepted filesystem plan snapshot, with node selection, outputs, acceptance checks, attempt limits, finite repair routes, zoom/scroll and optional route effects. Motion honors the OS preference. The inspector moves below the canvas on narrower screens. The panel clearly identifies preview scope; execution controls remain future work. File viewing/editing/downloads remain available below the graph. All interface text uses paired English/Simplified Chinese catalog keys.

## Loop current-position effects

Loop Design exposes live preparation progress before a generated plan exists. A status strip names the current stage and shows accepted-stage completion. A bright halo and current label identify the controller-owned node; a silver-white reflection sweeps only the incoming route during actual native work. Waiting for questions or approvals uses an amber stationary route, failures use red, and paused or uncertain state stops motion. Selected nodes have a separate inspection outline. Locate current and follow current keep the actual position accessible while allowing independent inspection. OS reduced motion, a hidden document, or an offscreen panel suppress presentation animation.

Current and completed preparation routes are solid; unstarted routes and failure feedback are dashed. The active solid silver route carries a tapered reflective band with a soft bloom, without moving dots. A slow metallic border reflection complements the current node. Light and dark palettes keep readable status text; completed routes remain quiet teal. The legend follows the same styles, and the effects toggle preserves the line distinction.

Once the Loop plan is ready, users can switch between preparation progress and development preview. Planned development nodes stay not started until an execution backend exists. This increment changes presentation and reads existing workflow/session projections; it does not add a simulated execution mode or mutate workflow state.


## Project and requirement AI models

The Settings icon in the sidebar footer opens a main-area page with an AI models section. The user maintains a shared list by entering an agent, display name and native model identifier for each row. Add/remove actions edit a draft; Save persists the list in SQLite. A reload action restores the latest saved list, and stale-save conflicts keep the local draft. The list starts empty and contains no automatic native catalog or alias suggestions. Display names can be Chinese or English.

Open a project's AI models tab to choose provider-specific defaults from the shared list. Open a requirement's AI conversation and use its model button to inherit project choices or select requirement-specific overrides. Scope selectors retain Agent default and link to Settings; custom identifiers are entered centrally. Previously saved identifiers remain selectable when removed from the shared list. Renaming/removing an entry does not rewrite current scope choices or conversation history. Native availability remains account/provider-owned.

Changes to scope choices apply to the next conversation turn or preparation stage. Current tasks keep their captured model. The right-side settings page and anchored requirement panel preserve sidebar hierarchy and tab order. Save status/errors are localized. Model panels close when leaving the requirement view, including keyboard navigation. English and Simplified Chinese copy update without losing model drafts.
