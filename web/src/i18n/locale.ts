import { en, zhCN } from './catalog.ts'
import type { TranslationKey } from './catalog.ts'

export type Locale = 'en' | 'zh-CN'
export const languageStorageKey = 'loom.language'
export type Parameters = Record<string, string | number>

export function resolveLocale(languages: readonly string[]): Locale {
  for (const language of languages) {
    const normalized = language.toLowerCase().replaceAll('_', '-')
    if (/^zh(?:-|$)/.test(normalized)) return 'zh-CN'
    if (/^en(?:-|$)/.test(normalized)) return 'en'
  }
  return 'en'
}

export function isPreference(value: unknown): value is Locale {
  return value === 'en' || value === 'zh-CN'
}

export function readPreference(storage?: Pick<Storage, 'getItem'>): Locale | undefined {
  try {
    const saved = storage?.getItem(languageStorageKey)
    return isPreference(saved) ? saved : undefined
  } catch { return undefined }
}

export function savePreference(preference: Locale, storage?: Pick<Storage, 'setItem'>) {
  try { storage?.setItem(languageStorageKey, preference) } catch { /* Language switching also works without browser storage. */ }
}

export function translate(locale: Locale, key: TranslationKey, parameters: Parameters = {}): string {
  const template = (locale === 'zh-CN' ? zhCN : en)[key]
  return template.replace(/\{(\w+)\}/g, (placeholder, name: string) => String(parameters[name] ?? placeholder))
}

const states: Record<string, TranslationKey> = Object.fromEntries(
  Object.keys(en).filter(key => key.startsWith('status.') && key !== 'status.other').map(key => [key.slice(7), key as TranslationKey]),
)

export function translateState(locale: Locale, state: unknown): string {
  const value = String(state ?? 'unknown')
  const key = states[value === 'inProgress' ? 'running' : value]
  return key ? translate(locale, key) : translate(locale, 'status.other', { state: value })
}

// Match controller-owned English messages, including persisted legacy records.
// User text, agent responses, commands, and unknown upstream diagnostics stay verbatim.
const messages = new Map<string, TranslationKey>(
  Object.entries(en).filter(([key]) => key.startsWith('errors.') || key.startsWith('discovery.') || key === 'local.directoryError').map(([key, message]) => [message, key as TranslationKey]),
)
messages.set('Codex cannot restore an empty session after its connection closes. Create a new session.', 'errors.emptySession')
messages.set('Codex returned no session identifier', 'errors.noSessionId')
messages.set('Codex returned a mismatched session identifier', 'errors.wrongSessionId')
messages.set('Could not resolve the default model. Choose an explicit model and retry.', 'errors.modelDefault')
messages.set('Failed to fetch', 'errors.network')
messages.set('Load failed', 'errors.network')
messages.set('NetworkError when attempting to fetch resource.', 'errors.network')
messages.set('connection closed', 'errors.connectionClosed')

const codes: Record<string, TranslationKey> = {
  invalid_model_catalog: 'errors.invalidModelCatalog', model_catalog_conflict: 'errors.modelCatalogConflict', invalid_models: 'models.invalid',
  branch_group_invalid: 'errors.branchGroupInvalid', branch_group_owned: 'errors.branchGroupOwned', branch_group_required: 'errors.branchGroupRequired', branch_member_invalid: 'errors.branchMemberInvalid', branch_member_not_found: 'errors.branchMemberNotFound', branch_member_required: 'errors.branchMemberRequired',
  skill_conflict: 'errors.skillConflict', invalid_skill: 'errors.invalidSkill', invalid_skill_name: 'errors.invalidSkillName', skill_read_only: 'skills.linkedReadOnly', skill_confirmation: 'errors.skillConfirmation', skills_unavailable: 'errors.skillsMutation',
  workflow_conflict: 'workflow.conflict', workflow_busy: 'workflow.busy', workflow_stale: 'workflow.stale', invalid_workflow: 'workflow.actionError', workflow_not_found: 'workflow.actionError', material_conflict: 'errors.materialConflict', material_not_found: 'errors.materialNotFound', materials_index: 'errors.materialIndex', invalid_material: 'errors.invalidMaterial', materials_unavailable: 'errors.materialsUnavailable', materials_remote: 'materials.localOnly', workspace_writer_busy: 'errors.worktreeWriter', workspace_not_ready: 'errors.worktreeNotReady', workspace_unavailable: 'errors.gitWorktreeUnavailable',
  local_scanning: 'errors.localScanning', local_missing: 'errors.localMissing', local_managed: 'errors.localManaged', local_directory: 'local.directoryError', local_unconfirmed: 'errors.localUnconfirmed',
  request_failed: 'errors.request', not_found: 'errors.notFound', agent_offline: 'errors.agentOffline',
  invalid_session: 'errors.invalidSession', invalid_title: 'errors.invalidTitle', invalid_task: 'errors.invalidTask',
  request_conflict: 'errors.requestConflict', session_busy: 'errors.sessionBusy', sync_required: 'errors.syncRequired',
  sync_in_progress: 'errors.syncInProgress', interaction_closed: 'errors.interactionClosed', interaction_expired: 'errors.interactionExpired',
  invalid_question: 'errors.invalidQuestion', invalid_answer: 'errors.invalidAnswer', invalid_decision: 'errors.invalidDecision',
  invalid_address: 'errors.invalidAddress', invalid_endpoint: 'errors.invalidEndpoint', invalid_token: 'errors.invalidToken',
  duplicate_endpoint: 'errors.duplicateEndpoint', invalid_cursor: 'errors.invalidCursor', invalid_json: 'errors.invalidJson',
  origin_rejected: 'errors.origin', json_required: 'errors.jsonRequired',
}

export function translateMessage(locale: Locale, message: string, code?: string): string {
  const key = messages.get(message) ?? (Object.hasOwn(en, message) ? message as TranslationKey : undefined)
  if (key) return translate(locale, key)
  if (message.startsWith('Codex handshake failed: ')) {
    return translate(locale, 'errors.handshake', { message: translateMessage(locale, message.slice(24)) })
  }
  if (message.startsWith('persist controller state: ')) {
    return translate(locale, 'errors.persist', { message: translateMessage(locale, message.slice(26)) })
  }
  const acknowledgement = message.match(/^(\S+) acknowledgement unavailable: (.+)$/)
  if (acknowledgement) return translate(locale, 'errors.ackUnavailable', { operation: acknowledgement[1], message: translateMessage(locale, acknowledgement[2]) })
  if (code && codes[code]) return translate(locale, codes[code])
  return message
}

export function translateError(locale: Locale, cause: unknown, fallback: TranslationKey): string {
  if (cause && typeof cause === 'object' && 'message' in cause && typeof cause.message === 'string') {
    const code = 'code' in cause && typeof cause.code === 'string' ? cause.code : undefined
    return translateMessage(locale, cause.message, code)
  }
  return translate(locale, fallback)
}

export function translateSessionTitle(locale: Locale, session: { title: string; defaultTitle?: boolean }): string {
  return (session.defaultTitle ?? session.title === 'New session') ? translate(locale, 'session.new') : session.title
}
