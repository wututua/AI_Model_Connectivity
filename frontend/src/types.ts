export interface ModelResult {
  provider_id: string
  provider_name: string
  provider_type: string
  provider_logo: string
  model: string
  current_model: string
  is_current: boolean
  status: string
  status_label: string
  status_class: string
  latency_ms: number
  response_preview: string
  error: string
  history: string[]
  show_curve_chart: boolean
  svg_path_line: string
  svg_path_area: string
  time_labels: string[]
  avg_latency_24h: string
  p50_latency_24h: string
  p95_latency_24h: string
  p99_latency_24h: string
  latency_samples_24h: number
  weekly_success_text: string
  availability: string
}

export interface ProviderReport {
  provider_id: string
  provider_type: string
  provider_name: string
  provider_logo: string
  current_model: string
  results: ModelResult[]
  ok_count: number
  slow_count: number
  error_count: number
  status: string
  status_label: string
  model_count: number
}

export interface ProviderError {
  provider_id: string
  provider_type: string
  error: string
}

export interface Report {
  title: string
  generated_at: string
  elapsed_ms: number
  global_concurrency: number
  provider_concurrency: number
  total: number
  ok_count: number
  slow_count: number
  error_count: number
  provider_count: number
  providers: ProviderReport[]
  provider_errors: ProviderError[]
  overall_status: string
  overall_class: string
  history_size: number
  stats_window_days: number
  theme: string
  theme_label: string
}

export interface RunningState {
  running: boolean
  task_id: number
  kind: string
  provider_id: string
  auto_check_interval_min_hours: number
  auto_check_interval_max_hours: number
  first_use: boolean
}

export interface RuntimeSettings {
  dashboard_title: string
  timeout_seconds: number
  model_list_timeout_seconds: number
  slow_threshold_ms: number
  concurrency: number
  provider_concurrency: number
  max_models_per_provider: number
  skip_models: string[]
  enable_history: boolean
  show_curve_chart: boolean
  stats_window_days: number
  history_size: number
  max_history_records: number
  show_error_detail: boolean
  theme_mode: string
  day_mode_start_hour: number
  day_mode_end_hour: number
  auto_check_interval_min_hours: number
  auto_check_interval_max_hours: number
  notify_platform: string
  notify_webhook_url: string
  notify_telegram_bot_token: string
  notify_telegram_chat_id: string
  notify_on_recovery: boolean
  notify_cooldown_minutes: number
  notify_providers: string[]
  notify_models: string[]
}

export interface SafeProviderConfig {
  id: string
  name: string
  type: string
  base_url: string
  models: string[]
  enabled: boolean
  probe_enabled: boolean
  api_key_set: boolean
}

export interface ProviderUpdate {
  id: string
  name: string
  type: string
  base_url: string
  api_key: string
  clear_api_key: boolean
  models: string[]
  enabled: boolean
  probe_enabled: boolean
}

export interface AdminConfig {
  settings: RuntimeSettings
  providers: SafeProviderConfig[]
}

export interface CheckTask {
  id: number
  kind: string
  status: string
  provider_id: string
  started_at: string
  finished_at: string
  elapsed_ms: number
  ok_count: number
  slow_count: number
  error_count: number
  total: number
  error_message: string
  report_generated_at: string
}

export interface ConfigExport {
  settings: RuntimeSettings
  providers: SafeProviderConfig[]
}

export interface ConfigImport {
  settings: RuntimeSettings
  providers: ProviderUpdate[]
}
