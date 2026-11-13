import type { RequirementWorkflow } from '../types'

export const preparationStages = ['requirements', 'design', 'grill', 'alignment', 'confirmation', 'loop'] as const
export type PreparationStage = typeof preparationStages[number]
export type LoopVisualState = 'pending' | 'queued' | 'running' | 'waiting' | 'completed' | 'failed' | 'paused' | 'unknown'
export type LoopProgressStatus = LoopVisualState | 'ready' | 'approval' | 'answers' | 'confirmation' | 'validating' | 'pausing'

// Inspection selection never supplies execution state. Preparation uses only the
// durable controller ledger and its native session; development stays a preview.
export function loopProgress(workflow: RequirementWorkflow | null, sessionState?: string) {
  const states = new Map<PreparationStage, LoopVisualState>(preparationStages.map(stage => [stage, 'pending']))
  if (!workflow) return { states, current: null, completed: 0, status: 'pending' as LoopProgressStatus, flowing: false }
  const state = workflow.state === 'cancelled' ? workflow.pausedState : workflow.state
  const current = state === 'review' ? 'confirmation' : workflow.stage
  const index = preparationStages.indexOf(current as PreparationStage)
  if (index < 0) return { states, current: null, completed: 0, status: 'unknown' as LoopProgressStatus, flowing: false }
  const accepted: Record<PreparationStage, boolean> = {
    requirements: Boolean(workflow.hashes['documents/requirements.md']),
    design: Boolean(workflow.hashes['design-documents/design.md']),
    grill: Boolean(workflow.hashes['decisions/grill-me.md']),
    alignment: index > 3 && Object.keys(workflow.hashes).length === 3,
    confirmation: index > 4,
    loop: false,
  }
  for (const stage of preparationStages.slice(0, index)) if (accepted[stage]) states.set(stage, 'completed')
  let visual: LoopVisualState = 'unknown'
  let status: LoopProgressStatus = 'unknown'
  if (state === 'preview') {
    for (const stage of preparationStages) states.set(stage, 'completed')
    visual = 'completed'; status = 'ready'
  } else if (state === 'pending') { visual = 'queued'; status = 'queued' }
  else if (state === 'questions') { visual = 'waiting'; status = 'answers' }
  else if (state === 'review') { visual = 'waiting'; status = 'confirmation' }
  else if (state === 'failed') { visual = 'failed'; status = 'failed' }
  else if (state === 'running') {
    if (sessionState === 'waiting_approval') { visual = 'waiting'; status = 'approval' }
    else if (sessionState === 'waiting_input') { visual = 'waiting'; status = 'answers' }
    else if (sessionState === 'syncing' || !sessionState) { visual = 'unknown'; status = 'unknown' }
    else if (sessionState === 'idle') { visual = 'running'; status = 'validating' }
    else if (sessionState === 'running') { visual = 'running'; status = 'running' }
  }
  if (workflow.state === 'cancelled') {
    visual = 'paused'
    status = workflow.pausedState === 'running' && ['running', 'waiting_approval', 'waiting_input'].includes(sessionState ?? '') ? 'pausing' : 'paused'
  }
  if (state !== 'preview') states.set(current as PreparationStage, visual)
  return { states, current: current as PreparationStage, completed: [...states.values()].filter(value => value === 'completed').length, status, flowing: visual === 'running' }
}
