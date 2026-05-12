// Minimal types for the Argon theme — only the fields the Argon pages render.
// Mirrors the backend JSON shape; kept independent of the default theme.

export interface ModelResult {
  provider_id: string
  provider_name: string
  model: string
  status: string
  status_label: string
  latency_ms: number
  error: string
  avg_latency_24h: string
  availability: string
}

export interface ProviderReport {
  provider_id: string
  provider_name: string
  provider_logo: string
  results: ModelResult[]
  ok_count: number
  slow_count: number
  error_count: number
  status: string
  status_label: string
  model_count: number
}

export interface Report {
  title: string
  generated_at: string
  elapsed_ms: number
  total: number
  ok_count: number
  slow_count: number
  error_count: number
  provider_count: number
  providers: ProviderReport[]
  overall_status: string
  overall_class: string
}

export interface RunningState {
  running: boolean
  first_use: boolean
}
