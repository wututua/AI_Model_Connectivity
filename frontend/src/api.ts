import type {
  AdminConfig,
  BillingSummary,
  CheckTask,
  ConfigExport,
  ConfigImport,
  Report,
  RunningState,
  RuntimeSettings,
  SafeProviderConfig,
  ProviderUpdate,
} from './types'

const TOKEN_KEY = 'cg_admin_token'

export function getToken(): string {
  return localStorage.getItem(TOKEN_KEY) ?? ''
}

export function setToken(token: string) {
  if (token) {
    localStorage.setItem(TOKEN_KEY, token)
  } else {
    localStorage.removeItem(TOKEN_KEY)
  }
}

function adminHeaders(): Record<string, string> {
  const token = getToken()
  const headers: Record<string, string> = { 'Content-Type': 'application/json' }
  if (token) headers['Authorization'] = `Bearer ${token}`
  return headers
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(path, {
    method,
    headers: adminHeaders(),
    body: body != null ? JSON.stringify(body) : undefined,
  })
  return decodeResponse<T>(res)
}

async function decodeResponse<T>(res: Response): Promise<T> {
  const data = await res.json().catch(() => null)
  if (res.status === 401 && location.pathname.startsWith('/admin')) {
    setToken('')
    window.dispatchEvent(new Event('cg:unauthorized'))
  }
  if (!res.ok) throw new Error((data as { error?: string } | null)?.error ?? `HTTP ${res.status}`)
  if (data === null) throw new Error('Invalid JSON response')
  return data as T
}

export const api = {
  status: (): Promise<Report> =>
    fetch('/api/status').then(decodeResponse<Report>),

  detection: (): Promise<RunningState> =>
    request<RunningState>('GET', '/api/admin/detection'),
  changeToken: (token: string): Promise<void> =>
    request('POST', '/api/admin/token', { token }),
  getViewToken: (): Promise<{ ok: boolean; token: string }> =>
    request('GET', '/api/admin/view-token'),
  rotateViewToken: (token?: string): Promise<{ ok: boolean; token: string }> =>
    request('POST', '/api/admin/view-token', token ? { token } : {}),
  revokeViewToken: (): Promise<{ ok: boolean }> =>
    request('DELETE', '/api/admin/view-token'),
  startDetection: (): Promise<unknown> =>
    request('POST', '/api/admin/detection/start'),
  stopDetection: (): Promise<unknown> =>
    request('POST', '/api/admin/detection/stop'),
  triggerCheck: (): Promise<unknown> =>
    request('POST', '/api/admin/check'),

  config: (): Promise<AdminConfig> =>
    request<AdminConfig>('GET', '/api/admin/config'),
  updateSettings: (settings: RuntimeSettings): Promise<AdminConfig> =>
    request<AdminConfig>('PUT', '/api/admin/settings', settings),

  providers: (): Promise<SafeProviderConfig[]> =>
    request<SafeProviderConfig[]>('GET', '/api/admin/providers'),
  createProvider: (p: ProviderUpdate): Promise<SafeProviderConfig> =>
    request<SafeProviderConfig>('POST', '/api/admin/providers', p),
  updateProvider: (id: string, p: ProviderUpdate): Promise<SafeProviderConfig> =>
    request<SafeProviderConfig>('PUT', `/api/admin/providers/${encodeURIComponent(id)}`, p),
  deleteProvider: (id: string): Promise<unknown> =>
    request('DELETE', `/api/admin/providers/${encodeURIComponent(id)}`),
  rerunProvider: (id: string): Promise<unknown> =>
    request('POST', `/api/admin/providers/${encodeURIComponent(id)}/rerun`),

  tasks: (params?: { limit?: number; offset?: number; status?: string }): Promise<CheckTask[]> => {
    const qs = new URLSearchParams()
    if (params?.limit) qs.set('limit', String(params.limit))
    if (params?.offset) qs.set('offset', String(params.offset))
    if (params?.status) qs.set('status', params.status)
    return request<CheckTask[]>('GET', `/api/admin/tasks?${qs}`)
  },

  billing: (days = 30): Promise<BillingSummary> =>
    request<BillingSummary>('GET', `/api/admin/billing?days=${days}`),

  exportConfig: (): Promise<ConfigExport> =>
    request<ConfigExport>('GET', '/api/admin/config/export'),
  importConfig: (data: ConfigImport): Promise<AdminConfig> =>
    request<AdminConfig>('POST', '/api/admin/config/import', data),
  reloadConfig: (): Promise<AdminConfig> =>
    request<AdminConfig>('POST', '/api/admin/config/reload'),
}
