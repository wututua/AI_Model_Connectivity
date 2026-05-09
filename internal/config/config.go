package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	AppHost                   string
	AppPort                   int
	WebDir                    string
	DataDir                   string
	DashboardTitle            string
	TimeoutSeconds            float64
	ModelListTimeoutSeconds   float64
	SlowThresholdMS           int
	Concurrency               int
	ProviderConcurrency       int
	MaxModelsPerProvider      int
	SkipModels                []string
	ProbePrompt               string
	ProbeSystemPrompt         string
	EnableHistory             bool
	ShowCurveChart            bool
	StatsWindowDays           int
	HistorySize               int
	MaxHistoryRecords         int
	ShowErrorDetail           bool
	ThemeMode                 string
	DayModeStartHour          int
	DayModeEndHour            int
	AutoCheckIntervalMinHours float64
	AutoCheckIntervalMaxHours float64
	AutoCheckRunOnStart       bool
	AdminToken                string
	Providers                 []ProviderConfig
}

type ProviderConfig struct {
	ID      string
	Name    string
	Type    string
	BaseURL string
	APIKey  string
	Models  []string
	Enabled bool
}

func Load(path string) (Config, error) {
	cfg := defaults()
	values := map[string]string{}
	if path == "" {
		path = ".env"
	}
	if err := readEnvFile(path, values); err != nil && !os.IsNotExist(err) {
		return cfg, err
	}
	for _, item := range os.Environ() {
		parts := strings.SplitN(item, "=", 2)
		if len(parts) == 2 {
			values[parts[0]] = parts[1]
		}
	}

	cfg.AppHost = getString(values, "APP_HOST", cfg.AppHost)
	cfg.AppPort = getInt(values, "APP_PORT", cfg.AppPort)
	cfg.WebDir = cleanPath(getString(values, "WEB_DIR", cfg.WebDir))
	cfg.DataDir = cleanPath(getString(values, "DATA_DIR", cfg.DataDir))
	cfg.DashboardTitle = getString(values, "DASHBOARD_TITLE", cfg.DashboardTitle)
	cfg.TimeoutSeconds = getFloat(values, "TIMEOUT_SECONDS", cfg.TimeoutSeconds)
	cfg.ModelListTimeoutSeconds = getFloat(values, "MODEL_LIST_TIMEOUT_SECONDS", cfg.ModelListTimeoutSeconds)
	cfg.SlowThresholdMS = getInt(values, "SLOW_THRESHOLD_MS", cfg.SlowThresholdMS)
	cfg.Concurrency = max(1, getInt(values, "CONCURRENCY", cfg.Concurrency))
	cfg.ProviderConcurrency = max(1, getInt(values, "PROVIDER_CONCURRENCY", cfg.ProviderConcurrency))
	cfg.MaxModelsPerProvider = max(0, getInt(values, "MAX_MODELS_PER_PROVIDER", cfg.MaxModelsPerProvider))
	cfg.SkipModels = splitList(getString(values, "SKIP_MODELS", ""))
	cfg.ProbePrompt = getString(values, "PROBE_PROMPT", cfg.ProbePrompt)
	cfg.ProbeSystemPrompt = getString(values, "PROBE_SYSTEM_PROMPT", cfg.ProbeSystemPrompt)
	cfg.EnableHistory = getBool(values, "ENABLE_HISTORY", cfg.EnableHistory)
	cfg.ShowCurveChart = getBool(values, "SHOW_CURVE_CHART", cfg.ShowCurveChart)
	cfg.StatsWindowDays = max(1, getInt(values, "STATS_WINDOW_DAYS", cfg.StatsWindowDays))
	cfg.HistorySize = max(1, getInt(values, "HISTORY_SIZE", cfg.HistorySize))
	cfg.MaxHistoryRecords = max(1, getInt(values, "MAX_HISTORY_RECORDS", cfg.MaxHistoryRecords))
	cfg.ShowErrorDetail = getBool(values, "SHOW_ERROR_DETAIL", cfg.ShowErrorDetail)
	cfg.ThemeMode = getString(values, "THEME_MODE", cfg.ThemeMode)
	cfg.DayModeStartHour = clamp(getInt(values, "DAY_MODE_START_HOUR", cfg.DayModeStartHour), 0, 23)
	cfg.DayModeEndHour = clamp(getInt(values, "DAY_MODE_END_HOUR", cfg.DayModeEndHour), 0, 23)
	cfg.AutoCheckIntervalMinHours = getFloat(values, "AUTO_CHECK_INTERVAL_MIN_HOURS", cfg.AutoCheckIntervalMinHours)
	cfg.AutoCheckIntervalMaxHours = getFloat(values, "AUTO_CHECK_INTERVAL_MAX_HOURS", cfg.AutoCheckIntervalMaxHours)
	cfg.AutoCheckRunOnStart = getBool(values, "AUTO_CHECK_RUN_ON_START", cfg.AutoCheckRunOnStart)
	cfg.AdminToken = getString(values, "ADMIN_TOKEN", cfg.AdminToken)
	cfg.Providers = loadProviders(values)
	return cfg, nil
}

func defaults() Config {
	return Config{
		AppHost:                   "127.0.0.1",
		AppPort:                   8080,
		WebDir:                    "web",
		DataDir:                   "data",
		DashboardTitle:            "模型连通性",
		TimeoutSeconds:            30,
		ModelListTimeoutSeconds:   20,
		SlowThresholdMS:           8000,
		Concurrency:               3,
		ProviderConcurrency:       1,
		ProbePrompt:               "只回复 OK 两个字母。",
		ProbeSystemPrompt:         "你是一个模型连通性探针。请只回复 OK，不要解释。",
		EnableHistory:             true,
		ShowCurveChart:            true,
		StatsWindowDays:           7,
		HistorySize:               30,
		MaxHistoryRecords:         500,
		ShowErrorDetail:           true,
		ThemeMode:                 "auto",
		DayModeStartHour:          8,
		DayModeEndHour:            18,
		AutoCheckIntervalMinHours: 0,
		AutoCheckIntervalMaxHours: 0,
	}
}

func readEnvFile(path string, values map[string]string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		value = strings.Trim(value, "\"'")
		values[key] = value
	}
	return scanner.Err()
}

func loadProviders(values map[string]string) []ProviderConfig {
	providers := []ProviderConfig{}
	for i := 1; ; i++ {
		prefix := fmt.Sprintf("PROVIDER_%d_", i)
		id := getString(values, prefix+"ID", "")
		name := getString(values, prefix+"NAME", "")
		baseURL := strings.TrimRight(getString(values, prefix+"BASE_URL", ""), "/")
		models := splitList(getString(values, prefix+"MODELS", ""))
		apiKey := getString(values, prefix+"API_KEY", "")
		if id == "" && name == "" && baseURL == "" && len(models) == 0 && apiKey == "" {
			break
		}
		if id == "" {
			id = fmt.Sprintf("provider-%d", i)
		}
		if name == "" {
			name = id
		}
		providerType := strings.ToLower(getString(values, prefix+"TYPE", "openai"))
		providers = append(providers, ProviderConfig{
			ID:      id,
			Name:    name,
			Type:    providerType,
			BaseURL: baseURL,
			APIKey:  apiKey,
			Models:  models,
			Enabled: getBool(values, prefix+"ENABLED", true),
		})
	}
	return providers
}

func getString(values map[string]string, key, fallback string) string {
	if value, ok := values[key]; ok {
		return value
	}
	return fallback
}

func getInt(values map[string]string, key string, fallback int) int {
	value, ok := values[key]
	if !ok || strings.TrimSpace(value) == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return fallback
	}
	return parsed
}

func getFloat(values map[string]string, key string, fallback float64) float64 {
	value, ok := values[key]
	if !ok || strings.TrimSpace(value) == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func getBool(values map[string]string, key string, fallback bool) bool {
	value, ok := values[key]
	if !ok || strings.TrimSpace(value) == "" {
		return fallback
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return fallback
	}
}

func splitList(value string) []string {
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r' || r == ';'
	})
	result := []string{}
	seen := map[string]bool{}
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		result = append(result, item)
	}
	return result
}

func cleanPath(path string) string {
	if path == "" {
		return path
	}
	return filepath.Clean(path)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func clamp(value, minValue, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}
