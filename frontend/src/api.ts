import type {
  AdminConfig,
  AcceptedCheck,
  BillingSummary,
  CheckTask,
  ConfigExport,
  ConfigImport,
  Report,
  RunningState,
  RuntimeSettings,
  SafeProviderConfig,
  ProviderUpdate,
  ModelDiscoveryRequest,
  AuthSession, User, UserInput,
} from './types'

try { localStorage.removeItem('cg_admin_token') } catch { /* Storage may be disabled. */ }
let csrfToken = ''
export function applySession(session: AuthSession) { csrfToken = session.csrf_token }

async function request<T>(method: string, path: string, body?: unknown, silentAuth = false, signal?: AbortSignal): Promise<T> {
  const res = await fetch(path, {
    method,
    credentials: 'same-origin',
    signal,
    headers: { 'Content-Type': 'application/json', ...(method !== 'GET' && csrfToken ? { 'X-CSRF-Token': csrfToken } : {}) },
    body: body != null ? JSON.stringify(body) : undefined,
  })
  return decodeResponse<T>(res, silentAuth)
}

async function decodeResponse<T>(res: Response, silentAuth = false): Promise<T> {
  const data = await res.json().catch(() => null)
  if (res.status === 401 && !silentAuth) {
    csrfToken = ''
    window.dispatchEvent(new Event('cg:unauthorized'))
  }
  if (res.status === 403 && data?.code === 'password_change_required') window.dispatchEvent(new Event('cg:session-refresh'))
  if (!res.ok) throw new Error((data as { error?: string } | null)?.error ?? `HTTP ${res.status}`)
  if (data === null) throw new Error('Invalid JSON response')
  return data as T
}

async function sessionRequest(method: string, path: string, body?: unknown): Promise<AuthSession> {
  return request<AuthSession>(method, path, body, path.endsWith('/login'))
}

export const api = {
  session: () => sessionRequest('GET', '/api/auth/session'),
  login: (username: string, password: string) => sessionRequest('POST', '/api/auth/login', { username, password }),
  logout: async () => { await request('POST', '/api/auth/logout'); csrfToken = '' },
  changePassword: (current_password: string, password: string) => sessionRequest('POST', '/api/auth/password', { current_password, password }),
  users: () => request<User[]>('GET', '/api/admin/users'),
  createUser: (value: UserInput) => request<User>('POST', '/api/admin/users', value),
  updateUser: (id: number, value: UserInput) => request<User>('PUT', `/api/admin/users/${id}`, value),
  deleteUser: (id: number) => request('DELETE', `/api/admin/users/${id}`),
  status: (): Promise<Report> =>
    request<Report>('GET', '/api/status'),

  detection: (): Promise<RunningState> =>
    request<RunningState>('GET', '/api/admin/detection'),
  startDetection: () => request<AcceptedCheck>('POST', '/api/admin/detection/start'),
  triggerCheck: () => request<AcceptedCheck>('POST', '/api/admin/check'),

  config: (): Promise<AdminConfig> =>
    request<AdminConfig>('GET', '/api/admin/config'),
  updateSettings: (settings: RuntimeSettings): Promise<AdminConfig> =>
    request<AdminConfig>('PUT', '/api/admin/settings', settings),

  providers: (): Promise<SafeProviderConfig[]> =>
    request<SafeProviderConfig[]>('GET', '/api/admin/providers'),
  discoverModels: (query: ModelDiscoveryRequest, signal?: AbortSignal): Promise<string[]> =>
    request<string[]>('POST', '/api/admin/provider-models', query, false, signal),
  createProvider: (p: ProviderUpdate): Promise<SafeProviderConfig> =>
    request<SafeProviderConfig>('POST', '/api/admin/providers', p),
  updateProvider: (id: string, p: ProviderUpdate): Promise<SafeProviderConfig> =>
    request<SafeProviderConfig>('PUT', `/api/admin/providers/${encodeURIComponent(id)}`, p),
  deleteProvider: (id: string): Promise<unknown> =>
    request('DELETE', `/api/admin/providers/${encodeURIComponent(id)}`),
  rerunProvider: (id: string) =>
    request<AcceptedCheck>('POST', `/api/admin/providers/${encodeURIComponent(id)}/rerun`),

  task: (id: number) => request<CheckTask>('GET', `/api/admin/tasks/${id}`),

  tasks: (params?: { limit?: number; offset?: number; status?: string }, signal?: AbortSignal): Promise<CheckTask[]> => {
    const qs = new URLSearchParams()
    if (params?.limit) qs.set('limit', String(params.limit))
    if (params?.offset) qs.set('offset', String(params.offset))
    if (params?.status) qs.set('status', params.status)
    return request<CheckTask[]>('GET', `/api/admin/tasks?${qs}`, undefined, false, signal)
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
