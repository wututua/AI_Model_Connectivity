import type { Report, RunningState } from './types'

const TOKEN_KEY = 'cg_admin_token'

export function getToken(): string {
  return localStorage.getItem(TOKEN_KEY) ?? ''
}

export function setToken(token: string) {
  if (token) localStorage.setItem(TOKEN_KEY, token)
  else localStorage.removeItem(TOKEN_KEY)
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
  const data = await res.json()
  if (!res.ok) throw new Error((data as { error?: string }).error ?? `HTTP ${res.status}`)
  return data as T
}

export const api = {
  status: (): Promise<Report> =>
    fetch('/api/status').then(async r => {
      const data = await r.json()
      if (!r.ok) throw new Error((data as { error?: string }).error ?? `HTTP ${r.status}`)
      return data as Report
    }),
  detection: (): Promise<RunningState> => request<RunningState>('GET', '/api/admin/detection'),
  changeToken: (token: string): Promise<void> => request('POST', '/api/admin/token', { token }),
  // PUT /api/admin/settings is a full replace, so we GET first, mutate
  // only the admin_theme field, then PUT back.  Dashboard theme is
  // intentionally not touched here — the two are independent.
  updateAdminTheme: async (theme: string): Promise<void> => {
    const cfg = await request<{ settings: Record<string, unknown> }>('GET', '/api/admin/config')
    await request('PUT', '/api/admin/settings', { ...cfg.settings, admin_theme: theme })
  },
}

