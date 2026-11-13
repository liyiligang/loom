// Throwaway UI model: three Loop Design variants on /?prototype=loop&variant=.
// All documents, generation steps, branches, and execution events are simulated.
import { computed, onBeforeUnmount, ref } from 'vue'
import type { TranslationKey } from '../../i18n/catalog'

export type NodeId = 'sources' | 'plan' | 'build' | 'tests' | 'review' | 'gate' | 'repair' | 'delivery'
export type NodeState = 'ready' | 'running' | 'passed' | 'failed' | 'blocked' | 'waiting' | 'skipped'
export type DemoPhase = 'empty' | 'generating' | 'preview' | 'running' | 'paused' | 'waiting' | 'completed' | 'failed' | 'blocked'
export type Scenario = 'repair' | 'pass' | 'blocked'
export interface LoopNode {
  id: NodeId
  role: 'planner' | 'developer' | 'verifier' | 'controller' | 'human'
  title: TranslationKey
  description: TranslationKey
  task: string
  inputs: string[]
  outputs: string[]
  criteria: string[]
  branch: string
}
export interface LoopEdge { id: string; from: NodeId; to: NodeId; feedback?: boolean; label?: TranslationKey }

export const sampleNodes: LoopNode[] = [
  { id: 'sources', role: 'planner', title: 'loopPrototype.node.sources', description: 'loopPrototype.desc.sources', task: 'Freeze the agreed requirement and design versions. List acceptance criteria and unresolved decisions.', inputs: ['requirements.md · v3', 'design.md · v2'], outputs: ['source-manifest.json', 'acceptance-contract.json'], criteria: ['Every acceptance criterion references its source.', 'Unresolved decisions are listed before execution.'], branch: 'main' },
  { id: 'plan', role: 'planner', title: 'loopPrototype.node.plan', description: 'loopPrototype.desc.plan', task: 'Break console search and filtering into small tasks. Define dependencies, evidence, and bounded repair paths.', inputs: ['source-manifest.json', 'acceptance-contract.json'], outputs: ['loop.json', 'implementation tasks'], criteria: ['Every executable node has explicit inputs and outputs.', 'All acceptance criteria have a verification owner.'], branch: 'main' },
  { id: 'build', role: 'developer', title: 'loopPrototype.node.build', description: 'loopPrototype.desc.build', task: 'Implement console search and state filtering. Preserve existing pagination and permission behavior.', inputs: ['implementation tasks', 'design.md'], outputs: ['candidate commit', 'change summary'], criteria: ['Changes match the agreed scope.', 'The candidate revision is recorded before verification.'], branch: 'feat/console/implement-01' },
  { id: 'tests', role: 'verifier', title: 'loopPrototype.node.tests', description: 'loopPrototype.desc.tests', task: 'Verify search, filter combinations, empty results, pagination, and denied access against the same candidate.', inputs: ['candidate commit', 'test-cases.yaml', 'dataset.jsonl'], outputs: ['test-report.json', 'failing cases'], criteria: ['Required checks pass on the exact candidate.', 'Missing data is blocked, never reported as passed.'], branch: 'feat/console/implement-01' },
  { id: 'review', role: 'verifier', title: 'loopPrototype.node.review', description: 'loopPrototype.desc.review', task: 'Independently review the candidate for correctness, scope, and design consistency.', inputs: ['candidate commit', 'design.md'], outputs: ['review-report.json'], criteria: ['No unresolved blocking finding.', 'The reviewed revision matches the tested revision.'], branch: 'feat/console/implement-01' },
  { id: 'gate', role: 'controller', title: 'loopPrototype.node.gate', description: 'loopPrototype.desc.gate', task: 'Evaluate the evidence contract. Route pass to delivery, actionable failures to repair, and missing inputs to attention.', inputs: ['test-report.json', 'review-report.json'], outputs: ['acceptance verdict', 'next action'], criteria: ['All required evidence is present and current.', 'Pass, fail, blocked, and unknown are distinct outcomes.'], branch: 'feat/console/implement-01' },
  { id: 'repair', role: 'developer', title: 'loopPrototype.node.repair', description: 'loopPrototype.desc.repair', task: 'Fix the reported pagination regression without changing acceptance criteria. Create a new candidate and rerun affected checks.', inputs: ['failing cases', 'review findings', 'prior candidate'], outputs: ['repair patch', 'new candidate commit'], criteria: ['Fixes address recorded evidence.', 'A new candidate invalidates prior acceptance results.'], branch: 'feat/console/optimize-01' },
  { id: 'delivery', role: 'human', title: 'loopPrototype.node.delivery', description: 'loopPrototype.desc.delivery', task: 'Inspect the accepted candidate, recorded checks, and change summary. Choose the next delivery action explicitly.', inputs: ['accepted candidate', 'evidence bundle', 'change summary'], outputs: ['delivery decision'], criteria: ['The final candidate is explicitly selected.', 'Publication is a separate action from task completion.'], branch: 'feat/console/integrate-01' },
]
export const sampleEdges: LoopEdge[] = [
  { id: 'source-plan', from: 'sources', to: 'plan' }, { id: 'plan-build', from: 'plan', to: 'build' },
  { id: 'build-tests', from: 'build', to: 'tests' }, { id: 'build-review', from: 'build', to: 'review' },
  { id: 'tests-gate', from: 'tests', to: 'gate' }, { id: 'review-gate', from: 'review', to: 'gate' },
  { id: 'gate-delivery', from: 'gate', to: 'delivery', label: 'loopPrototype.edge.pass' },
  { id: 'gate-repair', from: 'gate', to: 'repair', feedback: true, label: 'loopPrototype.edge.fail' },
  { id: 'repair-build', from: 'repair', to: 'build', feedback: true, label: 'loopPrototype.edge.retry' },
]

interface Step { states: Partial<Record<NodeId, NodeState>>; focus: NodeId; event: TranslationKey; phase?: DemoPhase; attempt?: number }
const initialStates = () => Object.fromEntries(sampleNodes.map(node => [node.id, 'ready'])) as Record<NodeId, NodeState>

export function useLoopDemo() {
  const phase = ref<DemoPhase>('empty')
  const states = ref(initialStates())
  const selectedId = ref<NodeId>('build')
  const nodes = ref(sampleNodes.map(node => ({ ...node })))
  const generationStep = ref(0)
  const attempt = ref(1)
  const scenario = ref<Scenario>('repair')
  const maxAttempts = ref(3)
  const follow = ref(true)
  const events = ref<{ id: number; node: NodeId; label: TranslationKey; attempt: number }[]>([])
  const generated = computed(() => !['empty', 'generating'].includes(phase.value))
  const selected = computed(() => nodes.value.find(node => node.id === selectedId.value)!)
  const passed = computed(() => Object.values(states.value).filter(value => value === 'passed').length)
  const active = computed(() => nodes.value.filter(node => states.value[node.id] === 'running'))
  let timer: ReturnType<typeof setInterval> | undefined
  let queue: Step[] = []
  let eventId = 0

  function clearTimer() { if (timer) clearInterval(timer); timer = undefined }
  function reset() {
    clearTimer(); queue = []; phase.value = 'empty'; states.value = initialStates(); events.value = []
    generationStep.value = 0; attempt.value = 1; selectedId.value = 'build'
    follow.value = true
  }
  function generate() {
    reset(); phase.value = 'generating'
    timer = setInterval(() => {
      generationStep.value++
      if (generationStep.value === 4) { clearTimer(); phase.value = 'preview' }
    }, 650)
  }
  function record(step: Step) {
    Object.assign(states.value, step.states)
    if (follow.value) selectedId.value = step.focus
    if (step.attempt) attempt.value = step.attempt
    if (step.phase) phase.value = step.phase
    events.value.push({ id: ++eventId, node: step.focus, label: step.event, attempt: attempt.value })
  }
  function advance() {
    if (!['running', 'paused'].includes(phase.value)) return
    const step = queue.shift()
    if (step) record(step)
    if (!queue.length || !['running', 'paused'].includes(phase.value)) clearTimer()
  }
  function beginTimer() { clearTimer(); timer = setInterval(advance, 1600) }
  function verificationSteps(): Step[] {
    return [
      { states: { build: 'passed', tests: 'running', review: 'running' }, focus: 'tests', event: 'loopPrototype.event.verify' },
      { states: { tests: 'passed', review: 'passed', gate: 'running' }, focus: 'gate', event: 'loopPrototype.event.evidence' },
      { states: { gate: 'passed', delivery: 'waiting' }, focus: 'delivery', event: 'loopPrototype.event.waiting', phase: 'waiting' },
    ]
  }
  function run() {
    if (!['preview', 'completed', 'failed'].includes(phase.value)) return
    maxAttempts.value = Math.min(6, Math.max(1, Number(maxAttempts.value) || 3))
    states.value = initialStates(); events.value = []; attempt.value = 1; phase.value = 'running'
    queue = [
      { states: { sources: 'running' }, focus: 'sources', event: 'loopPrototype.event.sources' },
      { states: { sources: 'passed', plan: 'running' }, focus: 'plan', event: 'loopPrototype.event.plan' },
      { states: { plan: 'passed', build: 'running' }, focus: 'build', event: 'loopPrototype.event.build' },
    ]
    if (scenario.value === 'blocked') {
      queue.push({ states: { build: 'passed', tests: 'blocked', gate: 'blocked' }, focus: 'tests', event: 'loopPrototype.event.blocked', phase: 'blocked' })
    } else if (scenario.value === 'repair') {
      queue.push(
        { states: { build: 'passed', tests: 'running', review: 'running' }, focus: 'tests', event: 'loopPrototype.event.verify' },
        { states: { tests: 'failed', review: 'passed', gate: 'failed' }, focus: 'gate', event: 'loopPrototype.event.failed' },
      )
      if (maxAttempts.value < 2) queue.push({ states: { repair: 'blocked' }, focus: 'repair', event: 'loopPrototype.event.budget', phase: 'failed' })
      else queue.push(
        { states: { repair: 'running' }, focus: 'repair', event: 'loopPrototype.event.repair' },
        { states: { repair: 'passed', build: 'running', tests: 'ready', review: 'ready', gate: 'ready' }, focus: 'build', event: 'loopPrototype.event.retry', attempt: 2 },
        ...verificationSteps(),
      )
    } else { states.value.repair = 'skipped'; queue.push(...verificationSteps()) }
    advance(); beginTimer()
  }
  function pause() { if (phase.value === 'running') { clearTimer(); phase.value = 'paused' } }
  function resume() { if (phase.value === 'paused') { phase.value = 'running'; beginTimer() } }
  function resolveBlocked() {
    if (phase.value !== 'blocked') return
    phase.value = 'running'; states.value.gate = 'ready'; states.value.repair = 'skipped'
    queue = verificationSteps(); advance(); beginTimer()
  }
  function finish() {
    if (phase.value !== 'waiting') return
    record({ states: { delivery: 'passed' }, focus: 'delivery', event: 'loopPrototype.event.complete', phase: 'completed' })
  }
  function updateTask(value: string) { selected.value.task = value }
  const snapshot = computed(() => ({
    schemaVersion: 1, prototype: true, title: 'Console search and filters', sourceRevision: 'sample-v3-v2',
    maxAttempts: maxAttempts.value, scenario: scenario.value, phase: phase.value, attempt: attempt.value,
    sources: [{ path: 'requirements.md', revision: 'v3', hash: 'sample:6ef312' }, { path: 'design.md', revision: 'v2', hash: 'sample:85a02c' }],
    nodes: nodes.value.map(node => ({ id: node.id, kind: node.id, role: node.role, task: node.task, inputs: node.inputs, outputs: node.outputs, acceptance: node.criteria, proposedBranch: node.branch, state: states.value[node.id] })),
    edges: sampleEdges, events: events.value,
  }))
  onBeforeUnmount(clearTimer)
  return { phase, states, selectedId, selected, nodes, generationStep, attempt, scenario, maxAttempts, follow, events, generated, passed, active, snapshot, generate, reset, run, pause, resume, advance, resolveBlocked, finish, updateTask }
}
