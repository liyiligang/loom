import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { request } from '../api'
import { mergeDetail, refreshScope } from './sessionSnapshot'
import type { Bootstrap, Detail, DiscoveryRun, WorkspaceEvent } from '../types'

export function useWorkspace(sessionId?: () => string) {
  const route = useRoute()
  const snapshot = ref<Bootstrap>({ version: '0.1.0', cursor: 0, endpoints: [], projects: [], requirements: [], sessions: [], capabilities: { codex: true, claude: false, mdns: false } })
  const detail = ref<Detail>()
  const discovery = ref<DiscoveryRun>()
  const online = ref(false)
  const loading = ref(true)
  const error = ref('')
  const selectedId = computed(() => sessionId ? sessionId() : typeof route.params.id === 'string' ? route.params.id : '')
  let socket: WebSocket | undefined
  let disposed = false
  let reloadTimer: ReturnType<typeof setTimeout> | undefined
  let retryTimer: ReturnType<typeof setTimeout> | undefined
  let discoveryTimer: ReturnType<typeof setTimeout> | undefined
  let refreshing: Promise<void> | undefined
  let dirtyBootstrap = false
  let dirtyDetail = false
  let fullDetail = false
  let revision = 0

  async function loadDetail(full = true) {
    const id = selectedId.value
    const version = ++revision
    if (!id) { detail.value = undefined; return }
    try {
      const current = detail.value?.session.id === id ? detail.value : undefined
      const query = !full && current ? `?afterCursor=${current.cursor}` : ''
      const value = await request<Detail>(`/sessions/${id}${query}`)
      if (version === revision && selectedId.value === id) {
        detail.value = mergeDetail(current, value)
        const sessions = snapshot.value.sessions.filter(session => session.id !== id)
        snapshot.value.sessions = [...sessions, detail.value.session].sort((a, b) => b.updatedAt.localeCompare(a.updatedAt))
        error.value = ''
      }
    } catch (cause) { if (version === revision) error.value = cause instanceof Error ? cause.message : 'Could not load the session.' }
  }

  async function refresh(scope = { bootstrap: true, detail: true, full: true }) {
    dirtyBootstrap ||= scope.bootstrap
    dirtyDetail ||= scope.detail
    fullDetail ||= scope.full
    if (refreshing) return refreshing
    refreshing = (async () => {
      try {
        while ((dirtyBootstrap || dirtyDetail) && !disposed) {
          const bootstrap = dirtyBootstrap
          const loadSession = dirtyDetail
          const full = fullDetail
          dirtyBootstrap = dirtyDetail = fullDetail = false
          if (bootstrap) snapshot.value = await request<Bootstrap>('/bootstrap')
          error.value = ''
          if (loadSession) await loadDetail(full)
        }
      } finally { loading.value = false }
    })()
    try { await refreshing } finally { refreshing = undefined }
  }

  let scheduledScope = { bootstrap: false, detail: false, full: false }
  function scheduleRefresh(scope: typeof scheduledScope) {
    scheduledScope.bootstrap ||= scope.bootstrap
    scheduledScope.detail ||= scope.detail
    scheduledScope.full ||= scope.full
    if (reloadTimer || disposed) return
    reloadTimer = setTimeout(async () => {
      reloadTimer = undefined
      const scope = scheduledScope
      scheduledScope = { bootstrap: false, detail: false, full: false }
      try { await refresh(scope) } catch { online.value = false }
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
      socket.onmessage = message => {
        try { scheduleRefresh(refreshScope(JSON.parse(message.data) as WorkspaceEvent, selectedId.value)) }
        catch { socket?.close() }
      }
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
  onMounted(connect)
  onUnmounted(() => {
    disposed = true
    socket?.close()
    if (reloadTimer) clearTimeout(reloadTimer)
    if (retryTimer) clearTimeout(retryTimer)
    if (discoveryTimer) clearTimeout(discoveryTimer)
  })
  return { snapshot, detail, discovery, online, loading, error, refresh, discover, selectedId }
}
