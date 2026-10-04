import type { Report } from '../types'

interface StatusUpdates {
  fetchReport: () => Promise<Report>
  onReport: (report: Report) => void
  onError: (error: Error) => void
  onLive: (live: boolean) => void
  createSource?: (() => EventSource) | null
}

export function startStatusUpdates(options: StatusUpdates): () => void {
  const createSource = options.createSource === undefined
    ? (typeof window.EventSource === 'function' ? () => new EventSource('/api/events') : null)
    : options.createSource
  let closed = false
  let live = false
  let polling = false
  let revision = 0
  let pollDelay = 30_000
  let reconnectDelay = 30_000
  let pollTimer: ReturnType<typeof setTimeout> | undefined
  let reconnectTimer: ReturnType<typeof setTimeout> | undefined
  let source: EventSource | null = null

  const poll = async () => {
    pollTimer = undefined
    if (closed || live || polling) return
    polling = true
    const startedRevision = revision
    try {
      const value = await options.fetchReport()
      if (!closed && revision === startedRevision) options.onReport(value)
    } catch (error) {
      if (!closed && revision === startedRevision) options.onError(error as Error)
    } finally {
      polling = false
      if (!closed && !live && pollTimer === undefined) {
        pollTimer = setTimeout(poll, pollDelay)
        pollDelay = Math.min(pollDelay * 2, 120_000)
      }
    }
  }

  const disconnected = () => {
    if (closed) return
    live = false
    options.onLive(false)
    source?.close()
    source = null
    if (!polling && pollTimer === undefined) pollTimer = setTimeout(poll, 30_000)
    if (createSource && reconnectTimer === undefined) {
      reconnectTimer = setTimeout(() => {
        reconnectTimer = undefined
        reconnectDelay = Math.min(reconnectDelay * 2, 120_000)
        connect()
      }, reconnectDelay)
    }
  }

  const connect = () => {
    if (!createSource || closed) return
    try {
      const connectedSource = createSource()
      source = connectedSource
      connectedSource.addEventListener?.('auth-required', () => {
        if (closed || source !== connectedSource) return
        disconnected()
        window.dispatchEvent(new Event('cg:unauthorized'))
      })
      connectedSource.onmessage = event => {
        if (closed || source !== connectedSource) return
        try {
          const value = JSON.parse(event.data) as Report
          if (!value || !Array.isArray(value.providers) || !value.generated_at) throw new Error('Invalid status event')
          revision++
          live = true
          pollDelay = reconnectDelay = 30_000
          clearTimeout(pollTimer)
          clearTimeout(reconnectTimer)
          pollTimer = reconnectTimer = undefined
          options.onReport(value)
          options.onLive(true)
        } catch {
          disconnected()
        }
      }
      connectedSource.onerror = () => {
        if (source === connectedSource) disconnected()
      }
    } catch {
      disconnected()
    }
  }

  void poll()
  connect()
  return () => {
    closed = true
    clearTimeout(pollTimer)
    clearTimeout(reconnectTimer)
    source?.close()
  }
}
