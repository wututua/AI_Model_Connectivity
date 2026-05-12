package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
)

type RuntimeSettings struct {
	DashboardTitle            string   `json:"dashboard_title"`
	TimeoutSeconds            float64  `json:"timeout_seconds"`
	ModelListTimeoutSeconds   float64  `json:"model_list_timeout_seconds"`
	SlowThresholdMS           int      `json:"slow_threshold_ms"`
	Concurrency               int      `json:"concurrency"`
	ProviderConcurrency       int      `json:"provider_concurrency"`
	MaxModelsPerProvider      int      `json:"max_models_per_provider"`
	SkipModels                []string `json:"skip_models"`
	EnableHistory             bool     `json:"enable_history"`
	ShowCurveChart            bool     `json:"show_curve_chart"`
	StatsWindowDays           int      `json:"stats_window_days"`
	HistorySize               int      `json:"history_size"`
	MaxHistoryRecords         int      `json:"max_history_records"`
	ShowErrorDetail           bool     `json:"show_error_detail"`
	ThemeMode                 string   `json:"theme_mode"`
	ActiveTheme               string   `json:"active_theme"`
	DayModeStartHour          int      `json:"day_mode_start_hour"`
	DayModeEndHour            int      `json:"day_mode_end_hour"`
	AutoCheckIntervalMinHours float64  `json:"auto_check_interval_min_hours"`
	AutoCheckIntervalMaxHours float64  `json:"auto_check_interval_max_hours"`
	NotifyPlatform            string   `json:"notify_platform"`
	NotifyWebhookURL          string   `json:"notify_webhook_url,omitempty"`
	NotifyTelegramBotToken    string   `json:"notify_telegram_bot_token,omitempty"`
	NotifyTelegramChatID      string   `json:"notify_telegram_chat_id,omitempty"`
	NotifyOnRecovery          bool     `json:"notify_on_recovery"`
	NotifyCooldownMinutes     int      `json:"notify_cooldown_minutes"`
	NotifyProviders           []string `json:"notify_providers"`
	NotifyModels              []string `json:"notify_models"`
}

type SafeProviderConfig struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Type         string   `json:"type"`
	BaseURL      string   `json:"base_url"`
	Models       []string `json:"models"`
	Enabled      bool     `json:"enabled"`
	ProbeEnabled bool     `json:"probe_enabled"`
	APIKeySet    bool     `json:"api_key_set"`
}

type ProviderUpdate struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Type         string   `json:"type"`
	BaseURL      string   `json:"base_url"`
	APIKey       string   `json:"api_key"`
	ClearAPIKey  bool     `json:"clear_api_key"`
	Models       []string `json:"models"`
	Enabled      bool     `json:"enabled"`
	ProbeEnabled bool     `json:"probe_enabled"`
}

type RuntimeConfig struct {
	Settings  RuntimeSettings  `json:"settings"`
	Providers []ProviderConfig `json:"providers"`
}

type ConfigExport struct {
	Settings  RuntimeSettings      `json:"settings"`
	Providers []SafeProviderConfig `json:"providers"`
}

type ConfigImport struct {
	Settings  RuntimeSettings  `json:"settings"`
	Providers []ProviderUpdate `json:"providers"`
}

type AdminConfig struct {
	Settings  RuntimeSettings      `json:"settings"`
	Providers []SafeProviderConfig `json:"providers"`
}

func SettingsFromConfig(cfg Config) RuntimeSettings {
	return RuntimeSettings{
		DashboardTitle:            cfg.DashboardTitle,
		TimeoutSeconds:            cfg.TimeoutSeconds,
		ModelListTimeoutSeconds:   cfg.ModelListTimeoutSeconds,
		SlowThresholdMS:           cfg.SlowThresholdMS,
		Concurrency:               cfg.Concurrency,
		ProviderConcurrency:       cfg.ProviderConcurrency,
		MaxModelsPerProvider:      cfg.MaxModelsPerProvider,
		SkipModels:                append([]string(nil), cfg.SkipModels...),
		EnableHistory:             cfg.EnableHistory,
		ShowCurveChart:            cfg.ShowCurveChart,
		StatsWindowDays:           cfg.StatsWindowDays,
		HistorySize:               cfg.HistorySize,
		MaxHistoryRecords:         cfg.MaxHistoryRecords,
		ShowErrorDetail:           cfg.ShowErrorDetail,
		ThemeMode:                 cfg.ThemeMode,
		ActiveTheme:               cfg.ActiveTheme,
		DayModeStartHour:          cfg.DayModeStartHour,
		DayModeEndHour:            cfg.DayModeEndHour,
		AutoCheckIntervalMinHours: cfg.AutoCheckIntervalMinHours,
		AutoCheckIntervalMaxHours: cfg.AutoCheckIntervalMaxHours,
		NotifyPlatform:            cfg.NotifyPlatform,
		NotifyWebhookURL:          cfg.NotifyWebhookURL,
		NotifyTelegramBotToken:    cfg.NotifyTelegramBotToken,
		NotifyTelegramChatID:      cfg.NotifyTelegramChatID,
		NotifyOnRecovery:          cfg.NotifyOnRecovery,
		NotifyCooldownMinutes:     cfg.NotifyCooldownMinutes,
		NotifyProviders:           append([]string(nil), cfg.NotifyProviders...),
		NotifyModels:              append([]string(nil), cfg.NotifyModels...),
	}
}

func RuntimeConfigFromConfig(cfg Config) RuntimeConfig {
	providers := append([]ProviderConfig(nil), cfg.Providers...)
	return RuntimeConfig{Settings: SettingsFromConfig(cfg), Providers: providers}
}

func ApplyRuntimeConfig(base Config, runtime RuntimeConfig) Config {
	cfg := ApplyRuntimeSettings(base, runtime.Settings)
	cfg.Providers = append([]ProviderConfig(nil), runtime.Providers...)
	return cfg
}

func ApplyRuntimeSettings(cfg Config, settings RuntimeSettings) Config {
	cfg.DashboardTitle = settings.DashboardTitle
	cfg.TimeoutSeconds = settings.TimeoutSeconds
	cfg.ModelListTimeoutSeconds = settings.ModelListTimeoutSeconds
	cfg.SlowThresholdMS = settings.SlowThresholdMS
	cfg.Concurrency = max(1, settings.Concurrency)
	cfg.ProviderConcurrency = max(1, settings.ProviderConcurrency)
	cfg.MaxModelsPerProvider = max(0, settings.MaxModelsPerProvider)
	cfg.SkipModels = append([]string(nil), settings.SkipModels...)
	cfg.EnableHistory = settings.EnableHistory
	cfg.ShowCurveChart = settings.ShowCurveChart
	cfg.StatsWindowDays = max(1, settings.StatsWindowDays)
	cfg.HistorySize = max(1, settings.HistorySize)
	cfg.MaxHistoryRecords = max(1, settings.MaxHistoryRecords)
	cfg.ShowErrorDetail = settings.ShowErrorDetail
	cfg.ThemeMode = settings.ThemeMode
	cfg.ActiveTheme = strings.TrimSpace(settings.ActiveTheme)
	if cfg.ActiveTheme == "" {
		cfg.ActiveTheme = "default"
	}
	cfg.DayModeStartHour = clamp(settings.DayModeStartHour, 0, 23)
	cfg.DayModeEndHour = clamp(settings.DayModeEndHour, 0, 23)
	cfg.AutoCheckIntervalMinHours = settings.AutoCheckIntervalMinHours
	cfg.AutoCheckIntervalMaxHours = settings.AutoCheckIntervalMaxHours
	cfg.NotifyPlatform = strings.ToLower(strings.TrimSpace(settings.NotifyPlatform))
	cfg.NotifyWebhookURL = settings.NotifyWebhookURL
	cfg.NotifyTelegramBotToken = settings.NotifyTelegramBotToken
	cfg.NotifyTelegramChatID = settings.NotifyTelegramChatID
	cfg.NotifyOnRecovery = settings.NotifyOnRecovery
	cfg.NotifyCooldownMinutes = max(0, settings.NotifyCooldownMinutes)
	cfg.NotifyProviders = append([]string(nil), settings.NotifyProviders...)
	cfg.NotifyModels = append([]string(nil), settings.NotifyModels...)
	return cfg
}

func SafeProviders(providers []ProviderConfig) []SafeProviderConfig {
	result := make([]SafeProviderConfig, 0, len(providers))
	for _, provider := range providers {
		result = append(result, SafeProviderConfig{
			ID:           provider.ID,
			Name:         provider.Name,
			Type:         provider.Type,
			BaseURL:      provider.BaseURL,
			Models:       append([]string(nil), provider.Models...),
			Enabled:      provider.Enabled,
			ProbeEnabled: provider.ProbeEnabled,
			APIKeySet:    provider.APIKey != "",
		})
	}
	return result
}

func ApplyProviderUpdate(existing ProviderConfig, update ProviderUpdate) ProviderConfig {
	provider := ProviderConfig{
		ID:           strings.TrimSpace(update.ID),
		Name:         strings.TrimSpace(update.Name),
		Type:         strings.ToLower(strings.TrimSpace(update.Type)),
		BaseURL:      strings.TrimRight(strings.TrimSpace(update.BaseURL), "/"),
		APIKey:       existing.APIKey,
		Models:       append([]string(nil), update.Models...),
		Enabled:      update.Enabled,
		ProbeEnabled: update.ProbeEnabled,
	}
	if provider.ID == "" {
		provider.ID = existing.ID
	}
	if provider.Name == "" {
		provider.Name = provider.ID
	}
	if provider.Type == "" {
		provider.Type = "openai"
	}
	if update.ClearAPIKey {
		provider.APIKey = ""
	} else if update.APIKey != "" {
		provider.APIKey = update.APIKey
	}
	return provider
}

func ValidateRuntimeSettings(settings RuntimeSettings) error {
	if settings.TimeoutSeconds <= 0 {
		return errors.New("timeout_seconds must be greater than 0")
	}
	if settings.ModelListTimeoutSeconds <= 0 {
		return errors.New("model_list_timeout_seconds must be greater than 0")
	}
	if settings.SlowThresholdMS <= 0 {
		return errors.New("slow_threshold_ms must be greater than 0")
	}
	if settings.Concurrency <= 0 || settings.ProviderConcurrency <= 0 {
		return errors.New("concurrency values must be greater than 0")
	}
	if settings.StatsWindowDays <= 0 || settings.HistorySize <= 0 || settings.MaxHistoryRecords <= 0 {
		return errors.New("history values must be greater than 0")
	}
	if settings.DayModeStartHour < 0 || settings.DayModeStartHour > 23 || settings.DayModeEndHour < 0 || settings.DayModeEndHour > 23 {
		return errors.New("day mode hours must be between 0 and 23")
	}
	if settings.NotifyCooldownMinutes < 0 {
		return errors.New("notify_cooldown_minutes must be greater than or equal to 0")
	}
	return nil
}

func ValidateProviders(providers []ProviderConfig) error {
	seen := map[string]bool{}
	for _, provider := range providers {
		id := strings.TrimSpace(provider.ID)
		if id == "" {
			return errors.New("provider id is required")
		}
		key := strings.ToLower(id)
		if seen[key] {
			return errors.New("provider id must be unique")
		}
		seen[key] = true
		if provider.BaseURL != "" {
			if err := validateProviderURL(provider.BaseURL); err != nil {
				return fmt.Errorf("provider %q: %w", id, err)
			}
		}
	}
	return nil
}

// validateProviderURL rejects non-http(s) schemes and link-local addresses
// (169.254.x.x) that are commonly used as cloud instance metadata endpoints.
// Localhost and RFC-1918 ranges are intentionally allowed for self-hosted use.
func validateProviderURL(rawURL string) error {
	u, err := url.ParseRequestURI(rawURL)
	if err != nil {
		return fmt.Errorf("invalid base_url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("base_url scheme must be http or https, got %q", u.Scheme)
	}
	host := u.Hostname()
	if ip := net.ParseIP(host); ip != nil {
		// Block link-local (169.254.0.0/16) — cloud metadata SSRF vector
		if ip.IsLinkLocalUnicast() {
			return fmt.Errorf("base_url resolves to a link-local address which is not allowed")
		}
	}
	return nil
}

func AdminConfigFromConfig(cfg Config) AdminConfig {
	return AdminConfig{Settings: SettingsFromConfig(cfg), Providers: SafeProviders(cfg.Providers)}
}
