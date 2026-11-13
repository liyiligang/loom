import { onBeforeUnmount, ref, watch } from 'vue'
import { request } from '../api'
import type { WorkflowAction, WorkflowDetail } from '../types'

export function useRequirementWorkflow(id: () => string, revision: () => number | undefined) {
  const value = ref<WorkflowDetail>({ workflow: null, documents: [], plan: null, stale: false })
  const busy = ref(false)
  const loading = ref(true)
  const error = ref<unknown>()
  let generation = 0
  let disposed = false
  let pending: { key: string; id: string } | undefined
  async function refresh() {
    const version = ++generation
    try {
      const result = await request<WorkflowDetail>(`/requirements/${id()}/workflow`)
      if (!disposed && version === generation) { value.value = result; error.value = undefined }
    } catch (cause) { if (!disposed && version === generation) error.value = cause }
    finally { if (!disposed && version === generation) loading.value = false }
  }
  async function act(input: WorkflowAction) {
    if (busy.value || loading.value) return false
    busy.value = true
    error.value = undefined
    const body = { ...input, revision: value.value.workflow?.revision ?? 0 }
    const key = JSON.stringify(body)
    if (pending?.key !== key) pending = { key, id: crypto.randomUUID() }
    const version = ++generation
    try {
      const result = await request<WorkflowDetail>(`/requirements/${id()}/workflow/actions`, 'POST', { ...body, clientRequestId: pending.id })
      if (!disposed && version === generation) value.value = result
      pending = undefined
      return true
    } catch (cause) { error.value = cause; await refresh(); error.value = cause; return false }
    finally { busy.value = false }
  }
  watch([id, revision], () => { void refresh() }, { immediate: true })
  onBeforeUnmount(() => { disposed = true; generation++ })
  return { value, busy, loading, error, refresh, act }
}
