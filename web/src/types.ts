export interface Endpoint {
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

export interface Session {
  id: string
  endpointId: string
  providerId: string
  title: string
  cwd: string
  model?: string
  state: string
  activeTurn?: string
  error?: string
  createdAt: string
  updatedAt: string
}

export interface Turn {
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
}

export interface Bootstrap {
  version: string
  cursor: number
  endpoints: Endpoint[]
  sessions: Session[]
  capabilities: { codex: boolean; claude: boolean; mdns: boolean }
}

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
