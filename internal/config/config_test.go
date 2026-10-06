package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func clearConfigEnvironment(t *testing.T) {
	t.Helper()
	keys := []string{
		"APP_HOST", "APP_PORT", "WEB_DIR", "DATA_DIR", "DATABASE_PATH",
		"DASHBOARD_TITLE", "TIMEOUT_SECONDS", "MODEL_LIST_TIMEOUT_SECONDS",
		"SLOW_THRESHOLD_MS", "CONCURRENCY", "PROVIDER_CONCURRENCY",
		"MAX_MODELS_PER_PROVIDER", "SKIP_MODELS", "PROBE_PROMPT", "PROBE_SYSTEM_PROMPT",
		"ENABLE_HISTORY", "SHOW_CURVE_CHART", "STATS_WINDOW_DAYS", "HISTORY_SIZE",
		"MAX_HISTORY_RECORDS", "SHOW_ERROR_DETAIL", "THEME_MODE",
		"DAY_MODE_START_HOUR", "DAY_MODE_END_HOUR", "AUTO_CHECK_INTERVAL_MIN_HOURS",
		"AUTO_CHECK_INTERVAL_MAX_HOURS", "AUTO_CHECK_RUN_ON_START", "ADMIN_TOKEN",
		"ADMIN_USERNAME", "ADMIN_PASSWORD", "SECURE_COOKIES", "STATUS_LOGIN_REQUIRED",
		"NOTIFY_PLATFORM", "NOTIFY_WEBHOOK_URL", "NOTIFY_TELEGRAM_BOT_TOKEN",
		"NOTIFY_TELEGRAM_CHAT_ID", "NOTIFY_ON_RECOVERY", "NOTIFY_COOLDOWN_MINUTES",
		"NOTIFY_PROVIDERS", "NOTIFY_MODELS",
	}
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(key, "PROVIDER_") {
			keys = append(keys, key)
		}
	}
	for _, key := range keys {
		t.Setenv(key, "")
		if err := os.Unsetenv(key); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLoadDefaults(t *testing.T) {
	clearConfigEnvironment(t)
	t.Chdir(t.TempDir())
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AppHost != "127.0.0.1" || cfg.AppPort != 8080 || cfg.WebDir != "web" ||
		cfg.DataDir != "data" || cfg.DatabasePath != filepath.Join("data", "cg.sqlite") {
		t.Fatal("unexpected startup defaults")
	}
	if cfg.AdminUsername != "admin" || cfg.AdminPassword != "" || cfg.SecureCookies ||
		cfg.AutoCheckRunOnStart || len(cfg.Providers) != 0 {
		t.Fatal("unexpected first-start configuration")
	}
}

func TestLoadProcessEnvironment(t *testing.T) {
	clearConfigEnvironment(t)
	dir := t.TempDir()
	for key, value := range map[string]string{
		"APP_HOST":                "0.0.0.0",
		"APP_PORT":                "9091",
		"WEB_DIR":                 filepath.Join(dir, "web"),
		"DATA_DIR":                dir,
		"DATABASE_PATH":           filepath.Join(dir, "custom.sqlite"),
		"SECURE_COOKIES":          "true",
		"STATUS_LOGIN_REQUIRED":   "true",
		"ADMIN_USERNAME":          "operator",
		"ADMIN_PASSWORD":          "TestPassword123",
		"PROBE_PROMPT":            "custom ping",
		"PROBE_SYSTEM_PROMPT":     "custom system",
		"AUTO_CHECK_RUN_ON_START": "true",
		"DASHBOARD_TITLE":         "Environment title",
		"TIMEOUT_SECONDS":         "15",
		"PROVIDER_1_ID":           "fixture",
		"PROVIDER_1_NAME":         "Fixture",
		"PROVIDER_1_BASE_URL":     "https://example.invalid/v1/",
		"PROVIDER_1_API_KEY":      "fixture=secret",
		"PROVIDER_1_MODELS":       "model-a;model-b,model-a",
	} {
		t.Setenv(key, value)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AppHost != "0.0.0.0" || cfg.AppPort != 9091 || cfg.DataDir != dir ||
		cfg.WebDir != filepath.Join(dir, "web") || cfg.DatabasePath != filepath.Join(dir, "custom.sqlite") {
		t.Fatal("startup paths or listening address ignored process environment")
	}
	if !cfg.SecureCookies || !cfg.StatusLoginRequired || cfg.AdminUsername != "operator" ||
		cfg.AdminPassword != "TestPassword123" || !cfg.AutoCheckRunOnStart ||
		cfg.ProbePrompt != "custom ping" || cfg.ProbeSystemPrompt != "custom system" ||
		cfg.DashboardTitle != "Environment title" || cfg.TimeoutSeconds != 15 {
		t.Fatal("process settings were not loaded")
	}
	if len(cfg.Providers) != 1 {
		t.Fatal("initial provider was not loaded")
	}
	p := cfg.Providers[0]
	if p.ID != "fixture" || p.Name != "Fixture" || p.Type != "openai" ||
		p.BaseURL != "https://example.invalid/v1" || p.APIKey != "fixture=secret" ||
		strings.Join(p.Models, ",") != "model-a,model-b" || !p.Enabled || !p.ProbeEnabled {
		t.Fatal("initial provider values were not preserved")
	}
	t.Setenv("DATABASE_PATH", "")
	cfg, err = Load()
	if err != nil || cfg.DatabasePath != filepath.Join(dir, "cg.sqlite") {
		t.Fatal("empty database path did not use the configured data directory")
	}
}

func TestLoadIgnoresLegacyLocalConfig(t *testing.T) {
	clearConfigEnvironment(t)
	t.Chdir(t.TempDir())
	// Invalid legacy values must not affect startup or inject providers.
	const filename = ".env"
	if err := os.WriteFile(filename, []byte("APP_PORT=0\nDATABASE_PATH=wrong.sqlite\nPROVIDER_1_BASE_URL=http://169.254.169.254\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load()
	if err != nil || cfg.AppPort != 8080 || len(cfg.Providers) != 0 ||
		cfg.DatabasePath != filepath.Join("data", "cg.sqlite") {
		t.Fatal("legacy file affected startup")
	}
	if err := os.Remove(filename); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filename, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(); err != nil {
		t.Fatalf("startup tried to open a legacy path: %v", err)
	}
}

func TestLoadRejectsInvalidEnvironment(t *testing.T) {
	clearConfigEnvironment(t)
	for _, test := range []struct {
		key   string
		value string
	}{
		{"APP_PORT", "0"},
		{"APP_PORT", "65536"},
		{"TIMEOUT_SECONDS", "NaN"},
		{"PROVIDER_1_BASE_URL", "http://169.254.169.254"},
	} {
		t.Run(test.key+"="+test.value, func(t *testing.T) {
			t.Setenv(test.key, test.value)
			if _, err := Load(); err == nil {
				t.Fatal("invalid environment value accepted")
			}
		})
	}
}
