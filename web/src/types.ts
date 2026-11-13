export interface LocalAgent { provider: string; name: string; path?: string; version?: string; state: string; error?: string }
export interface StoredSecret { id: string; name: string; kind: 'text' | 'token' | 'accessKey'; createdAt: string; updatedAt: string }
export interface SecretValues { content?: string; token?: string; accessKeyId?: string; secretAccessKey?: string }
export interface LocalStatus { state: string; confirmed: boolean; agents: LocalAgent[]; defaultDirectory: string }
export interface DirectoryListing { path: string; parent: string; directories: { name: string; path: string }[]; limited: boolean }

export interface Endpoint {
  local?: boolean
  id: string
  name: string
  provider: string
  address: string
  hasToken: boolean
  autoConnect: boolean
  state: string
  error?: string
  version?: string
}

export type ModelPreferences = Partial<Record<'codex' | 'claude', string>>
export interface ConfiguredModel { provider: 'codex' | 'claude'; id: string; name: string }
export interface ModelCatalog { revision: number; models: ConfiguredModel[] }
export interface Project { models?: ModelPreferences; id: string; name: string; cwd: string; local: boolean; endpointId?: string; secretIds?: string[] }
export interface WorktreeBinding { repositoryRoot: string; commonDir: string; branch: string; baseRef: string; baseCommit: string; path: string; directory: string; newBranch: boolean; owned: boolean; state: string; error?: string }
export interface GitRepository { root: string; commonDir: string; currentBranch: string; branches: { name: string; path?: string }[] }
export interface BranchMember { id: string; workspace: WorktreeBinding }
export interface BranchGroup { id: string; prefix: string; repositoryRoot: string; commonDir: string; baseRef: string; baseCommit: string; members: BranchMember[]; materialBranchId?: string; explicitSelection: boolean }
export type AccessMode = 'workspace' | 'full'
export interface Requirement { models?: ModelPreferences; conversationAccessMode?: AccessMode; branchGroup?: BranchGroup; id: string; projectId: string; name: string; defaultName?: boolean; workspace?: WorktreeBinding; previousWorkspaces?: WorktreeBinding[] }
export type MaterialKind = 'documents' | 'decisions' | 'testCases' | 'datasets' | 'loopDesign' | 'designDocuments' | 'prototypes'
export interface MaterialScope { projectId: string; requirementId: string; directory: string; branch: string; commit: string; key: string; root: string }
export interface RequirementMaterial { id: string; kind: MaterialKind; path: string; title: string; hash: string; size: number; modifiedAt: string; editable: boolean; relatedIds: string[] }
export interface MaterialCatalog { scope: MaterialScope; files: RequirementMaterial[]; limited: boolean }
export interface MaterialDetail extends RequirementMaterial { scope: MaterialScope; content: string; truncated: boolean }
export interface ProjectSkill { id: string; name: string; description: string; paths: string[]; error?: string; editable: boolean; deletable: boolean; revision: string }
export interface ProjectSkillCatalog { projectId: string; directory: string; scopeKey: string; skills: ProjectSkill[]; limited: boolean; unreadable: boolean }
export interface ProjectSkillDetail extends ProjectSkill { content: string; source: string; hash: string; scopeKey: string }

export interface Session {
  modelManaged?: boolean
  accessMode?: AccessMode
  branchId?: string
  purpose?: 'requirement'
  materialDirectory?: string
  workspace?: WorktreeBinding
  requirementId: string
  id: string
  endpointId: string
  providerId: string
  title: string
  defaultTitle?: boolean
  cwd: string
  model?: string
  state: string
  activeTurn?: string
  error?: string
  createdAt: string
  updatedAt: string
}

export interface Turn {
  model?: string
  accessMode?: AccessMode
  id: string
  sessionId: string
  providerId?: string
  clientRequestId: string
  input: string
  state: string
  error?: string
  createdAt: string
}

export interface Entry {
  id: string
  sessionId: string
  turnId: string
  providerId?: string
  kind: string
  role?: string
  text: string
  status: string
  detail?: Record<string, unknown>
  truncated?: boolean
  createdAt: string
}

export interface Question {
  id: string
  header?: string
  question: string
  isSecret?: boolean
  options?: { label: string; description: string }[] | null
}

export interface Interaction {
  id: string
  sessionId: string
  turnId: string
  kind: string
  payload: { questions?: Question[]; command?: string; reason?: string; grantRoot?: string; itemId?: string }
  state: string
  answer?: Record<string, unknown>
}

export interface Detail {
  session: Session
  turns: Turn[]
  entries: Entry[]
  interactions: Interaction[]
  cursor: number
  partial?: boolean
}

export interface WorkspaceEvent {
  id: number
  type: string
  resource: string
}

export interface Bootstrap {
  modelCatalog?: ModelCatalog
	workflows?: RequirementWorkflow[]
  local?: LocalStatus
	writers?: { sessionId: string; directory: string }[]
  version: string
  cursor: number
  endpoints: Endpoint[]
  projects: Project[]
  requirements: Requirement[]
  sessions: Session[]
  capabilities: { codex: boolean; claude: boolean; mdns: boolean }
}

export interface RequirementWorkflow {
  id: string; requirementId: string; revision: number
  state: 'pending' | 'running' | 'questions' | 'review' | 'preview' | 'failed' | 'cancelled'
	pausedState?: string
  stage: 'requirements' | 'design' | 'grill' | 'alignment' | 'loop'
  attempt: number; brief: string; endpointId: string; sessionId?: string; turnId?: string
  requestId: string; lastActionId: string; scopeKey: string; commit: string
  criteria: string[]; questions: { id: string; question: string; reason: string }[]
  answers: Record<string, string>; hashes: Record<string, string>; reviewHash?: string; error?: string; updatedAt: string
}
export interface LoopNode { id: string; title: string; kind: 'entry' | 'task' | 'check' | 'exit'; description: string; acceptanceIds: string[]; outputs: string[]; checks: string[]; maxAttempts: number }
export interface LoopEdge { from: string; to: string; condition: 'success' | 'failure'; maxTraversals?: number }
export interface LoopPlan { schemaVersion: number; title: string; sourceHashes: Record<string, string>; nodes: LoopNode[]; edges: LoopEdge[] }
export interface WorkflowDetail { workflow: RequirementWorkflow | null; documents: { path: string; content: string; hash: string }[]; plan: LoopPlan | null; stale: boolean; warning?: string }
export interface WorkflowAction { action: 'start' | 'answers' | 'confirm' | 'revise' | 'retry' | 'cancel' | 'resume'; brief?: string; endpointId?: string; answers?: Record<string, string>; reviewHash?: string }

export interface DiscoveryRun {
  id: string
  state: string
  checked: number
  total: number
  limited: boolean
  candidates: { address: string; verified: boolean; note: string }[]
}

export interface Reply {
  decision?: string
  answers?: Record<string, { answers: string[] }>
}
