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
  NotificationDelivery,
  ModelTarget, RequestBudget, MetricsToken, IssuedMetricsToken,
  AuthSession, User, UserInput,
  SystemUpdateStatus, SystemUpdateCheck, SystemUpdateJob,
} from './types'
import type { MonitoringData, MonitoringSettings } from './monitoring'

try { localStorage.removeItem('cg_admin_token') } catch { /* Storage may be disabled. */ }
let csrfToken = ''
export function applySession(session: AuthSession) { csrfToken = session.csrf_token }

export class APIError extends Error {
  constructor(message: string, public readonly status: number) { super(message) }
}

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
  if (!res.ok) throw new APIError((data as { error?: string } | null)?.error ?? `HTTP ${res.status}`, res.status)
  if (data === null) throw new Error('Invalid JSON response')
  return data as T
}

async function sessionRequest(method: string, path: string, body?: unknown): Promise<AuthSession> {
  return request<AuthSession>(method, path, body, path.endsWith('/login'))
}

export const api = {
  monitoring: (signal?: AbortSignal) => request<MonitoringData>('GET', '/api/admin/monitoring', undefined, false, signal),
  saveMonitoring: (value: MonitoringSettings) => request<MonitoringSettings>('PUT', '/api/admin/monitoring/settings', value),
  monitoringAction: (action: 'backup' | 'verify' | 'approve' | 'ack' | 'test-rule', value?: unknown) => request('POST', `/api/admin/monitoring/${action}`, value),
  updateStatus: (signal?: AbortSignal) => request<SystemUpdateStatus>('GET', '/api/admin/updates', undefined, false, signal),
  resolveUpdate: (request_id: string, signal?: AbortSignal) =>
    request<SystemUpdateStatus>('POST', '/api/admin/updates/resolve', { request_id }, false, signal),
  checkUpdate: (channel: 'stable' | 'preview', signal?: AbortSignal) =>
    request<SystemUpdateCheck>('POST', '/api/admin/updates/check', { channel }, false, signal),
  startUpdate: (channel: 'stable' | 'preview', version: string, request_id: string, signal?: AbortSignal) =>
    request<SystemUpdateJob>('POST', '/api/admin/updates/start', { channel, version, request_id, confirm: true }, false, signal),
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
  stopDetection: () => request<{ stopped: boolean }>('POST', '/api/admin/detection/stop'),
  checkModels: (value: { targets?: ModelTarget[]; failed_only?: boolean; provider_id?: string }) =>
    request<AcceptedCheck>('POST', '/api/admin/detection/selected', value),
  budget: () => request<RequestBudget>('GET', '/api/admin/budget'),
  batchProviders: (ids: string[], action: string, group = '') =>
    request<AdminConfig>('POST', '/api/admin/providers/batch', { ids, action, group }),
  metricsTokens: () => request<MetricsToken[]>('GET', '/api/admin/metrics-tokens'),
  createMetricsToken: (name: string) => request<IssuedMetricsToken>('POST', '/api/admin/metrics-tokens', { name }),
  rotateMetricsToken: (id: number) => request<IssuedMetricsToken>('POST', `/api/admin/metrics-tokens/${id}/rotate`),
  revokeMetricsToken: (id: number) => request('DELETE', `/api/admin/metrics-tokens/${id}`),

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

  notifications: (params: { limit: number; offset: number; status: string }, signal?: AbortSignal) => {
    const query = new URLSearchParams({ limit: String(params.limit), offset: String(params.offset), status: params.status })
    return request<NotificationDelivery[]>('GET', `/api/admin/notifications?${query}`, undefined, false, signal)
  },
  testNotification: () => request<NotificationDelivery>('POST', '/api/admin/notifications/test'),
  retryNotification: (id: number) => request<NotificationDelivery>('POST', `/api/admin/notifications/${id}/retry`),

  exportConfig: (): Promise<ConfigExport> =>
    request<ConfigExport>('GET', '/api/admin/config/export'),
  importConfig: (data: ConfigImport): Promise<AdminConfig> =>
    request<AdminConfig>('POST', '/api/admin/config/import', data),
}

export async function downloadAdminFile(path: string, filename: string) {
  const response = await fetch(path, { credentials: 'same-origin' })
  if (!response.ok) { await decodeResponse(response); return }
  const url = URL.createObjectURL(await response.blob())
  const link = document.createElement('a')
  link.href = url; link.download = filename
  document.body.appendChild(link); link.click(); link.remove()
  setTimeout(() => URL.revokeObjectURL(url), 1000)
}
