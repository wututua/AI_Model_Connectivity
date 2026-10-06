import type { Report } from '../types'

interface StatusUpdates {
  fetchReport: () => Promise<Report>
  onReport: (report: Report) => void
  onError: (error: Error) => void
  onLive: (live: boolean) => void
  createSource?: (() => EventSource) | null
}

export interface StatusController {
  close: () => void
  refresh: () => Promise<void>
}

export function startStatusUpdates(options: StatusUpdates): StatusController {
  const createSource = options.createSource === undefined
    ? (typeof window.EventSource === 'function' ? () => new EventSource('/api/events') : null)
    : options.createSource
  let closed = false
  let live = false
  let pending = 0
  let revision = 0
  let pollDelay = 30_000
  let reconnectDelay = 30_000
  let pollTimer: ReturnType<typeof setTimeout> | undefined
  let reconnectTimer: ReturnType<typeof setTimeout> | undefined
  let source: EventSource | null = null

  const schedulePoll = () => {
    if (!closed && !live && pending === 0 && pollTimer === undefined) {
      pollTimer = setTimeout(poll, pollDelay)
      pollDelay = Math.min(pollDelay * 2, 120_000)
    }
  }

  const refresh = async () => {
    if (closed) return
    clearTimeout(pollTimer)
    pollTimer = undefined
    pending++
    const startedRevision = ++revision
    try {
      const value = await options.fetchReport()
      if (!closed && revision === startedRevision) options.onReport(value)
    } catch (error) {
      if (!closed && revision === startedRevision) options.onError(error as Error)
    } finally {
      pending--
      schedulePoll()
    }
  }

  const poll = () => {
    pollTimer = undefined
    if (!closed && !live && pending === 0) void refresh()
  }

  const disconnected = () => {
    if (closed) return
    live = false
    options.onLive(false)
    source?.close()
    source = null
    if (pending === 0 && pollTimer === undefined) pollTimer = setTimeout(poll, 30_000)
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
          if (!value || !Array.isArray(value.providers) || (!value.generated_at && value.state !== 'pending' && value.state !== 'unconfigured')) throw new Error('Invalid status event')
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

  poll()
  connect()
  return {
    refresh,
    close: () => {
      closed = true
      clearTimeout(pollTimer)
      clearTimeout(reconnectTimer)
      source?.close()
    },
  }
}
