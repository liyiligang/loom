import { computed, reactive } from 'vue'

const drafts = new Map<string, Record<string, string>>()
function draft(id: string): Record<string, string> {
  let value = drafts.get(id)
  if (!value) {
    const loaded: Record<string, string> = Object.create(null)
    try {
      const saved: unknown = JSON.parse(localStorage.getItem(`loom.workflow.answers.${id}`) ?? '{}')
      if (saved && typeof saved === 'object' && !Array.isArray(saved)) for (const [key, text] of Object.entries(saved)) if (typeof text === 'string' && text.length <= 4000) loaded[key] = text
    } catch { /* Answers stay usable when storage is unavailable or malformed. */ }
    value = reactive(loaded)
    drafts.set(id, value)
  }
  return value
}
export function useWorkflowAnswers(id: () => string) {
  const answers = computed(() => draft(id()))
  function save() { try { localStorage.setItem(`loom.workflow.answers.${id()}`, JSON.stringify(answers.value)) } catch { /* Browser storage does not gate submission. */ } }
  function clear() { try { localStorage.removeItem(`loom.workflow.answers.${id()}`) } catch { /* The backend already retains submitted answers. */ } }
  return { answers, save, clear }
}
