export interface AlertRule {
  catalog_changes?: boolean; backup_failures?: boolean; budget_alerts?: boolean
  id: string; name: string; enabled: boolean; provider_id: string; model: string
  platform: string; url?: string; token?: string; chat_id?: string; credentials_set?: boolean
  failure_threshold: number; recovery_threshold: number; cooldown_minutes: number
}
export interface ProviderSchedule {
  provider_id: string; interval_minutes: number; slow_threshold_ms: number
  maintenance_start: string; maintenance_end: string
}
export interface ModelPrice { provider_id: string; model: string; input_per_million: number; output_per_million: number }
export interface MonitoringSettings {
  version: number; backup_interval_hours: number; backup_keep: number; monthly_budget: number
  rules: AlertRule[]; schedules: ProviderSchedule[]; prices: ModelPrice[]
}
export interface Catalog {
  provider_id: string; revision: string; models: string[]; approved: string[]; added: string[]; removed: string[]; updated_at: string
}
export interface Backup { name: string; created_at: string; size: number; sha256: string; verified_at: string }
export interface Incident {
  id: number; provider_id: string; model: string; revision: string; status: string
  opened_at: string; last_seen_at: string; resolved_at: string; acknowledged_at: string; note: string
}
export interface DiagnosticRecord {
  id: number; provider_id: string; model: string; status: string; error_type: string
  checked_at: string; latency_ms: number; first_token_ms: number; capability: string; capability_status: string
  diagnostics?: { dns_ms?: number; connect_ms?: number; tls_ms?: number; first_byte_ms?: number; connection_reused: boolean; http_status?: number; request_id?: string; retry_after?: string }
}
export interface MonitoringData {
  settings: MonitoringSettings; catalogs: Catalog[]; backups: Backup[]; incidents: Incident[]; diagnostics: DiagnosticRecord[]
  catalog_events: { id: number; provider_id: string; added: string[]; removed: string[]; created_at: string }[]
  events: { id: number; kind: string; detail: string; created_at: string }[]
  schedules: { provider_id: string; next_at: string; interval_minutes: number }[]
  cost: {
    month: string; estimated_usd: number; budget_usd: number; budget_exceeded: boolean; unknown_probes: number
    items: { provider_id: string; model: string; estimated_usd: number; priced_probes: number; unknown_probes: number }[]
  }
}
