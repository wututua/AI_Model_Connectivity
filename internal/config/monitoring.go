package config

import (
	"errors"
	"math"
	"strings"
)

type MonitoringSettings struct {
	Version             int64              `json:"version"`
	BackupIntervalHours int                `json:"backup_interval_hours"`
	BackupKeep          int                `json:"backup_keep"`
	Rules               []AlertRule        `json:"rules"`
	Schedules           []ProviderSchedule `json:"schedules"`
	Prices              []ModelPrice       `json:"prices"`
	MonthlyBudget       float64            `json:"monthly_budget"`
}

type AlertRule struct {
	Generation        string `json:"generation"`
	CatalogChanges    bool   `json:"catalog_changes"`
	BackupFailures    bool   `json:"backup_failures"`
	BudgetAlerts      bool   `json:"budget_alerts"`
	ID                string `json:"id"`
	Name              string `json:"name"`
	Enabled           bool   `json:"enabled"`
	ProviderID        string `json:"provider_id"`
	Model             string `json:"model"`
	Platform          string `json:"platform"`
	URL               string `json:"url,omitempty"`
	Token             string `json:"token,omitempty"`
	ChatID            string `json:"chat_id,omitempty"`
	CredentialsSet    bool   `json:"credentials_set,omitempty"`
	FailureThreshold  int    `json:"failure_threshold"`
	RecoveryThreshold int    `json:"recovery_threshold"`
	CooldownMinutes   int    `json:"cooldown_minutes"`
}

type ProviderSchedule struct {
	ProviderID       string `json:"provider_id"`
	IntervalMinutes  int    `json:"interval_minutes"`
	SlowThresholdMS  int    `json:"slow_threshold_ms"`
	MaintenanceStart string `json:"maintenance_start"`
	MaintenanceEnd   string `json:"maintenance_end"`
}

type ModelPrice struct {
	ProviderID       string  `json:"provider_id"`
	Model            string  `json:"model"`
	InputPerMillion  float64 `json:"input_per_million"`
	OutputPerMillion float64 `json:"output_per_million"`
}

func (s MonitoringSettings) Validate() error {
	if s.BackupIntervalHours < 0 || s.BackupIntervalHours > 8760 || s.BackupKeep < 1 || s.BackupKeep > 100 {
		return errors.New("backup interval must be 0..8760 hours; retention must be 1..100")
	}
	if math.IsNaN(s.MonthlyBudget) || math.IsInf(s.MonthlyBudget, 0) || s.MonthlyBudget < 0 || s.MonthlyBudget > 1e9 {
		return errors.New("invalid monthly USD budget")
	}
	if len(s.Rules) > 50 || len(s.Schedules) > 1000 || len(s.Prices) > 5000 {
		return errors.New("too many monitoring settings")
	}
	seen := map[string]bool{}
	for _, r := range s.Rules {
		if r.ProviderID != "" && ValidateProviderID(r.ProviderID) != nil {
			return errors.New("invalid alert provider ID")
		}
		if ValidateProviderID(r.ID) != nil || seen[r.ID] || strings.TrimSpace(r.Name) == "" || len(r.Name) > 128 {
			return errors.New("alert rules require unique IDs and names")
		}
		seen[r.ID] = true
		if r.FailureThreshold < 1 || r.FailureThreshold > 100 || r.RecoveryThreshold < 1 || r.RecoveryThreshold > 100 || r.CooldownMinutes < 0 || r.CooldownMinutes > 525600 {
			return errors.New("invalid alert thresholds")
		}
		switch r.Platform {
		case "telegram":
			if r.Enabled && (r.Token == "" || r.ChatID == "") {
				return errors.New("telegram credentials required")
			}
		case "webhook", "discord", "bark", "wecom", "dingtalk":
			if r.Enabled || r.URL != "" {
				if err := ValidateWebhookURL(r.URL); err != nil {
					return err
				}
			}
		default:
			return errors.New("unsupported notification platform")
		}
		if len(r.Token) > 4096 || len(r.ChatID) > 256 || len(r.URL) > 4096 || len(r.Model) > 512 {
			return errors.New("alert rule too long")
		}
	}
	seen = map[string]bool{}
	for _, p := range s.Schedules {
		if ValidateProviderID(p.ProviderID) != nil || seen[p.ProviderID] || p.IntervalMinutes < 0 || p.IntervalMinutes > 525600 || p.SlowThresholdMS < 0 || p.SlowThresholdMS > 86400000 {
			return errors.New("invalid provider schedule")
		}
		seen[p.ProviderID] = true
		if err := (OperationsSettings{MaintenanceStart: p.MaintenanceStart, MaintenanceEnd: p.MaintenanceEnd}).Validate(); err != nil {
			return err
		}
	}
	prices := map[[2]string]bool{}
	for _, p := range s.Prices {
		key := [2]string{p.ProviderID, p.Model}
		if ValidateProviderID(p.ProviderID) != nil || strings.TrimSpace(p.Model) == "" || len(p.Model) > 512 || prices[key] {
			return errors.New("invalid or duplicate model price")
		}
		prices[key] = true
		for _, value := range []float64{p.InputPerMillion, p.OutputPerMillion} {
			if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 1e9 {
				return errors.New("invalid model price")
			}
		}
	}
	return nil
}

func (s MonitoringSettings) IndependentSchedules() bool {
	for _, item := range s.Schedules {
		if item.IntervalMinutes > 0 {
			return true
		}
	}
	return false
}

func (s MonitoringSettings) Safe() MonitoringSettings {
	s.Rules = append([]AlertRule{}, s.Rules...)
	for i := range s.Rules {
		r := &s.Rules[i]
		r.CredentialsSet = r.URL != "" || r.Token != ""
		r.URL, r.Token, r.ChatID = "", "", ""
	}
	return s
}

func (r AlertRule) Apply(cfg Config) Config {
	cfg.NotifyPlatform, cfg.NotifyWebhookURL = r.Platform, r.URL
	cfg.NotifyTelegramBotToken, cfg.NotifyTelegramChatID = r.Token, r.ChatID
	cfg.NotifyProviders, cfg.NotifyModels = nil, nil
	if r.ProviderID != "" {
		cfg.NotifyProviders = []string{r.ProviderID}
	}
	if r.Model != "" {
		cfg.NotifyModels = []string{r.Model}
	}
	cfg.NotifyFailureThreshold, cfg.NotifyRecoveryThreshold = r.FailureThreshold, r.RecoveryThreshold
	cfg.NotifyCooldownMinutes, cfg.NotifyOnRecovery = r.CooldownMinutes, true
	return cfg
}
