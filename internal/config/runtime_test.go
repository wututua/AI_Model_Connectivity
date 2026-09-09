package config

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func TestValidateProviderURL(t *testing.T) {
	cases := []struct {
		url     string
		wantErr bool
		desc    string
	}{
		{"https://api.openai.com/v1", false, "valid https"},
		{"http://localhost:11434/v1", false, "localhost allowed (Ollama)"},
		{"http://127.0.0.1:11434/v1", false, "loopback allowed"},
		{"http://192.168.1.100:8080/v1", false, "RFC1918 allowed (LAN server)"},
		{"http://10.0.0.1/v1", false, "RFC1918 10.x allowed"},
		{"http://169.254.169.254/latest/meta-data", true, "AWS metadata blocked"},
		{"http://169.254.0.1/v1", true, "link-local blocked"},
		{"ftp://example.com/v1", true, "non-http scheme rejected"},
		{"file:///etc/passwd", true, "file scheme rejected"},
		{"not a url", true, "garbage rejected"},
		{"", true, "empty url is invalid"},
	}
	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			err := validateProviderURL(tc.url)
			if (err != nil) != tc.wantErr {
				t.Errorf("validateProviderURL(%q) error=%v, wantErr=%v", tc.url, err, tc.wantErr)
			}
		})
	}
}

func TestRuntimeSettingsRejectUnsafeNumbers(t *testing.T) {
	for _, value := range []float64{math.NaN(), math.Inf(1), math.Inf(-1), -1, 1000000} {
		for _, field := range []string{"timeout", "model_list_timeout", "min_interval", "max_interval"} {
			settings := SettingsFromConfig(defaults())
			switch field {
			case "timeout":
				settings.TimeoutSeconds = value
			case "model_list_timeout":
				settings.ModelListTimeoutSeconds = value
			case "min_interval":
				settings.AutoCheckIntervalMinHours = value
			case "max_interval":
				settings.AutoCheckIntervalMaxHours = value
			}
			if err := ValidateRuntimeSettings(settings); err == nil {
				t.Errorf("accepted unsafe %s value %v", field, value)
			}
		}
	}
	if err := ValidateRuntimeSettings(SettingsFromConfig(defaults())); err != nil {
		t.Fatalf("default settings rejected: %v", err)
	}
}

func TestProviderURLSecretsAndWebhookQuery(t *testing.T) {
	for _, value := range []string{"https://example.test/v1?api_key=secret", "https://example.test/v1?", "https://example.test/v1#secret"} {
		if err := validateProviderURL(value); err == nil {
			t.Errorf("provider URL accepted: %s", value)
		}
	}
	if err := ValidateWebhookURL("https://example.test/hook?access_token=secret"); err != nil {
		t.Fatalf("webhook query rejected: %v", err)
	}
}

func TestProviderIDsRejectPathDelimiters(t *testing.T) {
	for _, value := range []string{"", ".", "..", "a/b", `a\b`, "a?b", "a#b", "a\nb", strings.Repeat("a", 129)} {
		if err := ValidateProviderID(value); err == nil {
			t.Errorf("provider ID accepted: %q", value)
		}
	}
	for _, value := range []string{"openai-main", "p_1", "v1.2", "custom:model", "percent%id"} {
		if err := ValidateProviderID(value); err != nil {
			t.Errorf("valid provider ID rejected: %q: %v", value, err)
		}
	}
}

func TestValidateProvidersRejectsDuplicateID(t *testing.T) {
	providers := []ProviderConfig{
		{ID: "p1", BaseURL: "https://api.openai.com/v1"},
		{ID: "P1", BaseURL: "https://api.openai.com/v1"},
	}
	if err := ValidateProviders(providers); err == nil {
		t.Fatal("expected error for duplicate provider id (case-insensitive)")
	}
}

func TestValidateProvidersRejectsBadURL(t *testing.T) {
	providers := []ProviderConfig{
		{ID: "evil", BaseURL: "http://169.254.169.254/latest/meta-data"},
	}
	if err := ValidateProviders(providers); err == nil {
		t.Fatal("expected error for link-local base_url")
	}
}

func TestAdminConfigDoesNotExposeNotificationSecrets(t *testing.T) {
	cfg := Config{
		NotifyWebhookURL:       "https://hooks.example.test/secret",
		NotifyTelegramBotToken: "123456:telegram-secret",
		NotifyTelegramChatID:   "987654",
	}
	admin := AdminConfigFromConfig(cfg)

	if admin.Settings.NotifyWebhookURL != "" || !admin.Settings.NotifyWebhookURLSet {
		t.Fatalf("webhook secret was not redacted: %+v", admin.Settings)
	}
	if admin.Settings.NotifyTelegramBotToken != "" || !admin.Settings.NotifyTelegramBotTokenSet {
		t.Fatalf("telegram bot token was not redacted: %+v", admin.Settings)
	}
	if admin.Settings.NotifyTelegramChatID != "" || !admin.Settings.NotifyTelegramChatIDSet {
		t.Fatalf("telegram chat id was not redacted: %+v", admin.Settings)
	}

	data, err := json.Marshal(admin)
	if err != nil {
		t.Fatalf("marshal admin config: %v", err)
	}
	if strings.Contains(string(data), "telegram-secret") || strings.Contains(string(data), "hooks.example.test") {
		t.Fatalf("serialized admin config contains a notification secret: %s", data)
	}
}
