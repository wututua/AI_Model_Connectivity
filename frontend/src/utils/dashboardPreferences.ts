export type SortMode = 'default' | 'status' | 'name' | 'latency' | 'models'
export type ViewMode = 'detailed' | 'compact'

export interface DashboardPreferences {
  sortBy: SortMode
  viewMode: ViewMode
}

const key = 'cg:dashboard-preferences:v1'
const defaults: DashboardPreferences = { sortBy: 'default', viewMode: 'detailed' }

export function readDashboardPreferences(): DashboardPreferences {
  try {
    const value = JSON.parse(localStorage.getItem(key) || '{}')
    return {
      sortBy: ['default', 'status', 'name', 'latency', 'models'].includes(value?.sortBy) ? value.sortBy : defaults.sortBy,
      viewMode: ['detailed', 'compact'].includes(value?.viewMode) ? value.viewMode : defaults.viewMode,
    }
  } catch { return { ...defaults } }
}

export function saveDashboardPreferences(value: DashboardPreferences) {
  try { localStorage.setItem(key, JSON.stringify(value)) } catch { /* Preferences remain usable without storage. */ }
}
