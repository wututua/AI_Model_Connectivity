package config

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// OperationsSettings is shared by persisted settings and the effective config.
type OperationsSettings struct {
	DailyRequestLimit       int    `json:"daily_request_limit"`
	DiscoveryModelLimit     int    `json:"discovery_model_limit"`
	NotifyFailureThreshold  int    `json:"notify_failure_threshold"`
	NotifyRecoveryThreshold int    `json:"notify_recovery_threshold"`
	MaintenanceStart        string `json:"maintenance_start"`
	MaintenanceEnd          string `json:"maintenance_end"`
}

type ProbeOptions struct {
	Capability       string  `json:"capability"`
	AssertContains   string  `json:"assert_contains"`
	AssertJSON       bool    `json:"assert_json"`
	AssertJSONKeys   string  `json:"assert_json_keys"`
	Protocol         string  `json:"protocol"`
	Stream           bool    `json:"stream"`
	MaxTokens        int     `json:"max_tokens"`
	TokenLimitField  string  `json:"token_limit_field"`
	OmitTemperature  bool    `json:"omit_temperature"`
	Temperature      float64 `json:"temperature"`
	TimeoutSeconds   float64 `json:"timeout_seconds"`
	Prompt           string  `json:"prompt"`
	SystemPrompt     string  `json:"system_prompt"`
	OmitSystemPrompt bool    `json:"omit_system_prompt"`
}

type ModelTarget struct {
	ProviderID string `json:"provider_id"`
	Model      string `json:"model"`
}

type CheckSelection struct {
	Targets    []ModelTarget `json:"targets"`
	FailedOnly bool          `json:"failed_only"`
	ProviderID string        `json:"provider_id"`
}

type ProviderBatch struct {
	IDs    []string `json:"ids"`
	Action string   `json:"action"`
	Group  string   `json:"group"`
}

func (p ProbeOptions) Validate() error {
	if p.Capability != "" && p.Capability != "text" && p.Capability != "tools" && p.Capability != "embedding" {
		return errors.New("unsupported capability")
	}
	if (p.Capability == "tools" || p.Capability == "embedding") && (p.Stream || p.Protocol != "" && p.Protocol != "chat") {
		return errors.New("tools and embedding probes require non-streaming Chat configuration")
	}
	if len(p.AssertContains) > 4096 || len(p.AssertJSONKeys) > 1024 {
		return errors.New("assertion too long")
	}
	switch p.Protocol {
	case "", "chat", "responses", "anthropic", "gemini":
	default:
		return errors.New("probe.protocol must be chat, responses, anthropic or gemini")
	}
	if (p.Protocol == "anthropic" || p.Protocol == "gemini") && p.TokenLimitField == "max_completion_tokens" {
		return errors.New("native protocols do not support max_completion_tokens")
	}
	if p.Protocol == "anthropic" && !p.OmitTemperature && p.Temperature > 1 {
		return errors.New("Anthropic temperature must be between 0 and 1")
	}
	if p.TokenLimitField != "" && p.TokenLimitField != "max_tokens" && p.TokenLimitField != "max_completion_tokens" {
		return errors.New("invalid probe.token_limit_field")
	}
	if p.MaxTokens < 0 || p.MaxTokens > 131072 {
		return errors.New("probe.max_tokens must be between 0 and 131072")
	}
	if !isFiniteNonNegative(p.Temperature) || p.Temperature > 2 {
		return errors.New("probe.temperature must be between 0 and 2")
	}
	if !isFiniteNonNegative(p.TimeoutSeconds) || p.TimeoutSeconds > 86400 {
		return errors.New("probe.timeout_seconds must be between 0 and 86400")
	}
	if len([]rune(p.Prompt)) > 4096 || len([]rune(p.SystemPrompt)) > 4096 {
		return errors.New("probe prompts must not exceed 4096 characters")
	}
	return nil
}

func (s OperationsSettings) Validate() error {
	if s.DailyRequestLimit < 0 || s.DailyRequestLimit > 10000000 ||
		s.DiscoveryModelLimit < 0 || s.DiscoveryModelLimit > 100000 {
		return errors.New("request and discovery limits are outside the supported range")
	}
	if s.NotifyFailureThreshold < 0 || s.NotifyFailureThreshold > 100 ||
		s.NotifyRecoveryThreshold < 0 || s.NotifyRecoveryThreshold > 100 {
		return errors.New("notification thresholds must be between 0 and 100")
	}
	if s.MaintenanceStart == "" && s.MaintenanceEnd == "" {
		return nil
	}
	start, err := time.Parse(time.RFC3339, s.MaintenanceStart)
	if err != nil {
		return errors.New("maintenance_start must include a time zone (RFC3339)")
	}
	end, err := time.Parse(time.RFC3339, s.MaintenanceEnd)
	if err != nil || !end.After(start) {
		return errors.New("maintenance_end must be after maintenance_start (RFC3339)")
	}
	return nil
}

func (s OperationsSettings) InMaintenance(now time.Time) bool {
	start, startErr := time.Parse(time.RFC3339, s.MaintenanceStart)
	end, endErr := time.Parse(time.RFC3339, s.MaintenanceEnd)
	return startErr == nil && endErr == nil && !now.Before(start) && now.Before(end)
}

func validateProviderMetadata(p ProviderConfig) error {
	if len([]rune(p.Group)) > 128 || len(p.Tags) > 20 {
		return errors.New("provider group or tags exceed the supported limits")
	}
	for _, tag := range p.Tags {
		if strings.TrimSpace(tag) == "" || len([]rune(tag)) > 64 {
			return errors.New("tags must contain 1 to 64 characters")
		}
	}
	if err := p.Probe.Validate(); err != nil {
		return fmt.Errorf("provider %q: %w", p.ID, err)
	}
	return nil
}
