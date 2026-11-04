import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { request } from '../api'
import type { Bootstrap, Detail, DiscoveryRun } from '../types'

export function useWorkspace() {
  const route = useRoute()
  const snapshot = ref<Bootstrap>({ version: '0.1.0', cursor: 0, endpoints: [], sessions: [], capabilities: { codex: true, claude: false, mdns: false } })
  const detail = ref<Detail>()
  const discovery = ref<DiscoveryRun>()
  const online = ref(false)
  const loading = ref(true)
  const error = ref('')
  const selectedId = computed(() => typeof route.params.id === 'string' ? route.params.id : '')
  let socket: WebSocket | undefined
  let disposed = false
  let reloadTimer: ReturnType<typeof setTimeout> | undefined
  let retryTimer: ReturnType<typeof setTimeout> | undefined
  let discoveryTimer: ReturnType<typeof setTimeout> | undefined
  let refreshing: Promise<void> | undefined
  let dirty = false
  let revision = 0

  async function loadDetail() {
    const id = selectedId.value
    const version = ++revision
    if (!id) { detail.value = undefined; return }
    try {
      const value = await request<Detail>(`/sessions/${id}`)
      if (version === revision && selectedId.value === id) { detail.value = value; error.value = '' }
    } catch (cause) { if (version === revision) error.value = cause instanceof Error ? cause.message : 'Could not load the session.' }
  }

  async function refresh() {
    dirty = true
    if (refreshing) return refreshing
    refreshing = (async () => {
      try {
        while (dirty && !disposed) {
          dirty = false
          snapshot.value = await request<Bootstrap>('/bootstrap')
          error.value = ''
          await loadDetail()
        }
      } finally { loading.value = false }
    })()
    try { await refreshing } finally { refreshing = undefined }
  }

  function scheduleRefresh() {
    if (reloadTimer || disposed) return
    reloadTimer = setTimeout(async () => {
      reloadTimer = undefined
      try { await refresh() } catch { online.value = false }
    }, 120)
  }

  async function connect() {
    if (disposed) return
    try {
      await refresh()
      if (disposed) return
      const address = new URL(`/api/v1/events?afterCursor=${snapshot.value.cursor}`, location.href)
      address.protocol = location.protocol === 'https:' ? 'wss:' : 'ws:'
      socket = new WebSocket(address)
      socket.onopen = () => { online.value = true }
      socket.onmessage = () => scheduleRefresh()
      socket.onclose = () => { online.value = false; if (!disposed) retryTimer = setTimeout(connect, 1500) }
      socket.onerror = () => socket?.close()
    } catch {
      online.value = false
      error.value = 'Loom is unreachable. Check that the server is running.'
      if (!disposed) retryTimer = setTimeout(connect, 1500)
    }
  }

  async function pollDiscovery(id: string) {
    if (disposed) return
    try {
      discovery.value = await request<DiscoveryRun>(`/discovery/${id}`)
      if (discovery.value.state === 'scanning') discoveryTimer = setTimeout(() => pollDiscovery(id), 800)
    } catch { /* A later discovery run can recover without affecting the conversation. */ }
  }

  async function discover() {
    if (discoveryTimer) clearTimeout(discoveryTimer)
    discovery.value = await request<DiscoveryRun>('/discovery', 'POST')
    void pollDiscovery(discovery.value.id)
  }

  watch(selectedId, () => { detail.value = undefined; void loadDetail() })
  onMounted(async () => { await connect(); if (!disposed && snapshot.value) { try { await discover() } catch { /* Connection status already explains server availability. */ } } })
  onUnmounted(() => {
    disposed = true
    socket?.close()
    if (reloadTimer) clearTimeout(reloadTimer)
    if (retryTimer) clearTimeout(retryTimer)
    if (discoveryTimer) clearTimeout(discoveryTimer)
  })
  return { snapshot, detail, discovery, online, loading, error, refresh, discover, selectedId }
}
