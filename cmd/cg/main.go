package main

import (
	"context"
	crand "crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	mathrand "math/rand"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"cg/internal/config"
	"cg/internal/metrics"
	"cg/internal/notify"
	"cg/internal/probe"
	"cg/internal/report"
	"cg/internal/storage"
	"cg/internal/web"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	baseCfg, err := config.Load(".env")
	if err != nil {
		slog.Error("load config", "err", err)
		os.Exit(1)
	}
	args := os.Args[1:]
	if len(args) > 0 && args[0] == "healthcheck" {
		if err := healthcheck(baseCfg.AppPort); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if web.IsPublicBindHost(baseCfg.AppHost) && strings.TrimSpace(baseCfg.AdminToken) == "" {
		slog.Error("public bind requires ADMIN_TOKEN to be explicitly configured")
		os.Exit(1)
	}
	store, err := storage.NewSQLite(context.Background(), baseCfg.DatabasePath, baseCfg.DataDir)
	if err != nil {
		slog.Error("open database", "err", err)
		os.Exit(1)
	}
	defer store.Close()
	runtimeCfg, ok, err := store.LoadRuntimeConfig(context.Background())
	if err != nil {
		slog.Error("load runtime config", "err", err)
		os.Exit(1)
	}
	cfg := baseCfg
	if ok {
		cfg = config.ApplyRuntimeConfig(baseCfg, runtimeCfg)
	} else {
		runtimeCfg = config.RuntimeConfigFromConfig(baseCfg)
		if err := store.SaveRuntimeConfig(context.Background(), runtimeCfg); err != nil {
			slog.Error("save runtime config", "err", err)
			os.Exit(1)
		}
	}

	broker := web.NewBroker()
	if err := config.ValidateRuntimeSettings(config.SettingsFromConfig(cfg)); err != nil {
		slog.Error("invalid persisted runtime settings", "err", err)
		os.Exit(1)
	}
	if err := config.ValidateProviders(cfg.Providers); err != nil {
		slog.Error("invalid persisted providers", "err", err)
		os.Exit(1)
	}
	app := &application{baseCfg: baseCfg, cfg: cfg, store: store, broker: broker, metrics: metrics.New(), schedulerWake: make(chan struct{}, 1)}

	if err := app.initializeTokens(context.Background()); err != nil {
		slog.Error("initialize authentication", "err", err)
		os.Exit(1)
	}
	if len(args) > 0 {
		switch args[0] {
		case "check", "once":
			if _, err := app.check(context.Background()); err != nil {
				slog.Error("check failed", "err", err)
				os.Exit(1)
			}
			return
		case "serve":
			// fall through
		default:
			slog.Error("unknown command", "cmd", args[0], "hint", "use serve or check")
			os.Exit(1)
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if cfg.AutoCheckRunOnStart {
		go func() {
			if _, err := app.checkWithOptions(ctx, checkOptions{Kind: "startup", SaveLatest: true}); err != nil {
				if errors.Is(err, web.ErrCheckAlreadyRunning) {
					slog.Warn("startup check skipped", "err", err)
					return
				}
				slog.Error("startup check failed", "err", err)
			}
		}()
	}
	go app.scheduler(ctx)

	srv := web.NewServer(cfg, store, app.check, broker, app)
	srv.SetMetrics(app.metrics)
	server := srv.HTTPServer()
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- server.ListenAndServe()
	}()

	select {
	case err := <-serverErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server error", "err", err)
			os.Exit(1)
		}
	case <-ctx.Done():
		app.StopCheck()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil && !errors.Is(err, context.DeadlineExceeded) {
			slog.Error("shutdown server", "err", err)
			os.Exit(1)
		}
		if err := server.Close(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("close server", "err", err)
			os.Exit(1)
		}
		if err := <-serverErr; err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server error", "err", err)
			os.Exit(1)
		}
	}
}

type application struct {
	baseCfg        config.Config
	cfg            config.Config
	store          *storage.SQLiteStore
	broker         *web.Broker
	metrics        *metrics.Metrics
	schedulerWake  chan struct{}
	mu             sync.RWMutex
	configMu       sync.Mutex
	tokenMu        sync.Mutex
	running        bool
	runCancel      context.CancelFunc
	taskID         int64
	taskKind       string
	taskProviderID string
	adminToken     string
	adminFirstUse  bool
	viewToken      string
}

type checkOptions struct {
	Kind       string
	ProviderID string
	SaveLatest bool
}

func (a *application) check(ctx context.Context) (report.Report, error) {
	return a.checkWithOptions(ctx, checkOptions{Kind: "manual", SaveLatest: true})
}

func (a *application) CheckProvider(ctx context.Context, providerID string) (report.Report, error) {
	return a.checkWithOptions(ctx, checkOptions{Kind: "provider", ProviderID: providerID, SaveLatest: true})
}

func (a *application) StopCheck() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.running || a.runCancel == nil {
		return false
	}
	a.runCancel()
	return true
}

func (a *application) RunningState() web.RunningState {
	a.mu.RLock()
	defer a.mu.RUnlock()
	cfg := a.cfg
	return web.RunningState{
		Running:                   a.running,
		TaskID:                    a.taskID,
		Kind:                      a.taskKind,
		ProviderID:                a.taskProviderID,
		AutoCheckIntervalMinHours: cfg.AutoCheckIntervalMinHours,
		AutoCheckIntervalMaxHours: cfg.AutoCheckIntervalMaxHours,
		FirstUse:                  a.adminFirstUse,
	}
}

func (a *application) AdminConfig(context.Context) (config.AdminConfig, error) {
	return config.AdminConfigFromConfig(a.currentConfig()), nil
}

func (a *application) AdminToken() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.adminToken
}

func (a *application) ChangeAdminToken(ctx context.Context, newToken string) error {
	a.tokenMu.Lock()
	defer a.tokenMu.Unlock()
	if a.currentConfig().AdminToken != "" {
		return errors.New("ADMIN_TOKEN is configured externally; update the environment or .env and restart")
	}
	if err := config.ValidateToken(newToken); err != nil {
		return err
	}
	if newToken == a.ViewToken() {
		return errors.New("admin token and view token must differ")
	}
	if err := a.store.SetKVs(ctx, map[string]string{"admin_stored_token": newToken, "admin_token_first_use": "false"}); err != nil {
		return err
	}
	a.mu.Lock()
	a.adminToken = newToken
	a.adminFirstUse = false
	a.mu.Unlock()
	return nil
}

func (a *application) ViewToken() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.viewToken
}

func (a *application) ChangeViewToken(ctx context.Context, newToken string) error {
	a.tokenMu.Lock()
	defer a.tokenMu.Unlock()
	if newToken != "" {
		if err := config.ValidateToken(newToken); err != nil {
			return err
		}
		if newToken == a.AdminToken() {
			return errors.New("admin token and view token must differ")
		}
	}
	if err := a.store.SetKV(ctx, "admin_view_token", newToken); err != nil {
		return err
	}
	a.mu.Lock()
	a.viewToken = newToken
	a.mu.Unlock()
	return nil
}

func (a *application) UpdateSettings(ctx context.Context, settings config.RuntimeSettings) (config.AdminConfig, error) {
	a.configMu.Lock()
	defer a.configMu.Unlock()
	current := a.currentConfig()
	mergeNotificationSecrets(&settings, current)
	if err := config.ValidateRuntimeSettings(settings); err != nil {
		return config.AdminConfig{}, err
	}
	runtimeCfg := config.RuntimeConfig{Settings: settings, Providers: current.Providers}
	if err := a.replaceRuntimeConfig(ctx, runtimeCfg); err != nil {
		return config.AdminConfig{}, err
	}
	return config.AdminConfigFromConfig(a.currentConfig()), nil
}

func (a *application) UpsertProvider(ctx context.Context, id string, update config.ProviderUpdate) (config.SafeProviderConfig, error) {
	a.configMu.Lock()
	defer a.configMu.Unlock()
	current := a.currentConfig()
	providers := append([]config.ProviderConfig(nil), current.Providers...)
	found := -1
	for i, provider := range providers {
		if provider.ID == id || (id == "" && provider.ID == update.ID) {
			found = i
			break
		}
	}
	existing := config.ProviderConfig{Enabled: true}
	if found >= 0 {
		existing = providers[found]
	}
	provider := config.ApplyProviderUpdate(existing, update)
	if id != "" {
		provider.ID = id
	}
	if found >= 0 {
		providers[found] = provider
	} else {
		providers = append(providers, provider)
	}
	if err := config.ValidateProviders(providers); err != nil {
		return config.SafeProviderConfig{}, err
	}
	runtimeCfg := config.RuntimeConfig{Settings: config.SettingsFromConfig(current), Providers: providers}
	if err := a.replaceRuntimeConfig(ctx, runtimeCfg); err != nil {
		return config.SafeProviderConfig{}, err
	}
	return config.SafeProviders([]config.ProviderConfig{provider})[0], nil
}

func (a *application) DeleteProvider(ctx context.Context, id string) error {
	a.configMu.Lock()
	defer a.configMu.Unlock()
	current := a.currentConfig()
	providers := []config.ProviderConfig{}
	found := false
	for _, provider := range current.Providers {
		if provider.ID == id {
			found = true
			continue
		}
		providers = append(providers, provider)
	}
	if !found {
		return fmt.Errorf("provider %q not found", id)
	}
	return a.replaceRuntimeConfig(ctx, config.RuntimeConfig{Settings: config.SettingsFromConfig(current), Providers: providers})
}

func (a *application) ExportConfig(context.Context) (config.ConfigExport, error) {
	current := a.currentConfig()
	return config.ConfigExport{Settings: config.AdminConfigFromConfig(current).Settings, Providers: config.SafeProviders(current.Providers)}, nil
}

func (a *application) ImportConfig(ctx context.Context, value config.ConfigImport) (config.AdminConfig, error) {
	a.configMu.Lock()
	defer a.configMu.Unlock()
	current := a.currentConfig()
	mergeNotificationSecrets(&value.Settings, current)
	if err := config.ValidateRuntimeSettings(value.Settings); err != nil {
		return config.AdminConfig{}, err
	}
	existing := map[string]config.ProviderConfig{}
	for _, provider := range current.Providers {
		existing[provider.ID] = provider
	}
	providers := make([]config.ProviderConfig, 0, len(value.Providers))
	for _, item := range value.Providers {
		providers = append(providers, config.ApplyProviderUpdate(existing[item.ID], item))
	}
	if err := config.ValidateProviders(providers); err != nil {
		return config.AdminConfig{}, err
	}
	if err := a.replaceRuntimeConfig(ctx, config.RuntimeConfig{Settings: value.Settings, Providers: providers}); err != nil {
		return config.AdminConfig{}, err
	}
	return config.AdminConfigFromConfig(a.currentConfig()), nil
}

func (a *application) ReloadConfig(ctx context.Context) (config.AdminConfig, error) {
	a.configMu.Lock()
	defer a.configMu.Unlock()
	loaded, err := config.Load(".env")
	if err != nil {
		return config.AdminConfig{}, err
	}
	current := a.currentConfig()
	if loaded.AppHost != current.AppHost || loaded.AppPort != current.AppPort || loaded.WebDir != current.WebDir || loaded.DatabasePath != current.DatabasePath || loaded.DataDir != current.DataDir || loaded.AdminToken != current.AdminToken {
		return config.AdminConfig{}, errors.New("changes to listening address, paths or ADMIN_TOKEN require a restart")
	}
	if web.IsPublicBindHost(loaded.AppHost) && loaded.AdminToken == "" {
		return config.AdminConfig{}, errors.New("public bind requires an explicitly configured ADMIN_TOKEN")
	}
	runtimeCfg, ok, err := a.store.LoadRuntimeConfig(ctx)
	if err != nil {
		return config.AdminConfig{}, err
	}
	cfg := loaded
	if ok {
		cfg = config.ApplyRuntimeConfig(loaded, runtimeCfg)
		if len(loaded.Providers) > 0 {
			cfg.Providers = append([]config.ProviderConfig(nil), loaded.Providers...)
			runtimeCfg.Providers = append([]config.ProviderConfig(nil), loaded.Providers...)
			if err := a.store.SaveRuntimeConfig(ctx, runtimeCfg); err != nil {
				return config.AdminConfig{}, err
			}
		}
	}
	a.mu.Lock()
	a.baseCfg = loaded
	a.cfg = cfg
	a.mu.Unlock()
	a.wakeScheduler()
	return config.AdminConfigFromConfig(cfg), nil
}

func (a *application) ListTasks(ctx context.Context, query storage.TaskQuery) ([]storage.CheckTask, error) {
	return a.store.ListCheckTasks(ctx, query)
}

func (a *application) GetTask(ctx context.Context, id int64) (storage.CheckTask, error) {
	return a.store.GetCheckTask(ctx, id)
}

func (a *application) currentConfig() config.Config {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.cfg
}

func (a *application) replaceRuntimeConfig(ctx context.Context, value config.RuntimeConfig) error {
	cfg := config.ApplyRuntimeConfig(a.baseCfg, value)
	if err := config.ValidateRuntimeSettings(value.Settings); err != nil {
		return err
	}
	if err := config.ValidateProviders(value.Providers); err != nil {
		return err
	}
	if err := a.store.SaveRuntimeConfig(ctx, value); err != nil {
		return err
	}
	a.mu.Lock()
	a.cfg = cfg
	a.mu.Unlock()
	a.wakeScheduler()
	return nil
}

func (a *application) wakeScheduler() {
	select {
	case a.schedulerWake <- struct{}{}:
	default:
	}
}

func (a *application) checkWithOptions(ctx context.Context, options checkOptions) (report.Report, error) {
	if options.Kind == "" {
		options.Kind = "manual"
	}
	started := time.Now()
	runCtx, cancel := context.WithCancel(ctx)

	a.mu.Lock()
	if a.running {
		a.mu.Unlock()
		cancel()
		return report.Report{}, web.ErrCheckAlreadyRunning
	}
	a.running = true
	a.runCancel = cancel
	a.taskKind = options.Kind
	a.taskProviderID = options.ProviderID
	a.mu.Unlock()

	taskID, err := a.store.CreateCheckTask(ctx, storage.CheckTask{Kind: options.Kind, Status: "running", ProviderID: options.ProviderID, StartedAt: started.Format(time.RFC3339)})
	if err != nil {
		a.finishRunState()
		cancel()
		return report.Report{}, err
	}
	a.mu.Lock()
	a.taskID = taskID
	a.mu.Unlock()

	value, runErr := a.runCheck(runCtx, options)
	finished := time.Now()
	status := "success"
	errorMessage := ""
	if runErr != nil {
		if errors.Is(runCtx.Err(), context.Canceled) {
			status = "canceled"
		} else {
			status = "error"
		}
		errorMessage = runErr.Error()
	}
	if err := a.store.FinishCheckTask(context.Background(), taskID, storage.CheckTaskUpdate{
		Status:            status,
		FinishedAt:        finished,
		ElapsedMS:         int(finished.Sub(started).Milliseconds()),
		OKCount:           value.OKCount,
		SlowCount:         value.SlowCount,
		ErrorCount:        value.ErrorCount,
		Total:             value.Total,
		ErrorMessage:      errorMessage,
		ReportGeneratedAt: value.GeneratedAt,
	}); err != nil {
		slog.Error("finish check task failed", "err", err)
	}
	if a.metrics != nil {
		a.metrics.RecordCheck(options.Kind, status, finished.Sub(started).Seconds())
	}
	a.finishRunState()
	cancel()
	return value, runErr
}

func (a *application) finishRunState() {
	a.mu.Lock()
	a.running = false
	a.runCancel = nil
	a.taskID = 0
	a.taskKind = ""
	a.taskProviderID = ""
	a.mu.Unlock()
}

func (a *application) runCheck(ctx context.Context, options checkOptions) (report.Report, error) {
	started := time.Now()
	cfg := a.currentConfig()
	if options.ProviderID != "" {
		filtered, ok := filterProvider(cfg, options.ProviderID)
		if !ok {
			return report.Report{}, fmt.Errorf("provider %q not found", options.ProviderID)
		}
		cfg = filtered
	}
	runner := probe.NewRunner(cfg)
	results, providerErrors, err := runner.Run(ctx)
	if err != nil {
		return report.Report{}, err
	}
	history := map[string][]report.HistoryRecord{}
	if cfg.EnableHistory {
		loaded, loadErr := a.store.LoadHistory(ctx, cfg.MaxHistoryRecords, cfg.StatsWindowDays)
		if loadErr != nil {
			slog.Warn("load history failed", "err", loadErr)
		} else {
			history = loaded
		}
	}
	value, _ := report.Build(cfg, results, providerErrors, history, started)
	if options.ProviderID != "" && options.SaveLatest {
		latest, latestErr := a.store.LatestReport(ctx)
		if latestErr != nil {
			slog.Warn("load latest report for provider rerun failed", "err", latestErr)
		} else {
			value = report.MergeProvider(latest, value, options.ProviderID)
		}
	}
	if a.metrics != nil {
		for _, result := range results {
			a.metrics.RecordProbe(result)
		}
	}
	var latest *report.Report
	if options.SaveLatest {
		latest = &value
	}
	if err := a.store.RecordCheck(ctx, results, time.Now(), cfg.MaxHistoryRecords, cfg.EnableHistory, latest); err != nil {
		return report.Report{}, fmt.Errorf("save probe results: %w", err)
	}
	if options.SaveLatest {
		if a.broker != nil {
			a.broker.Publish(value)
		}
		if err := notify.New(cfg, storage.SQLiteNotifyStateStore{Store: a.store}).SendIfNeeded(ctx, value); err != nil {
			slog.Warn("send notify failed", "err", err)
		}
	}
	slog.Info("check finished", "ok", value.OKCount, "slow", value.SlowCount, "error", value.ErrorCount, "total", value.Total)
	return value, nil
}

func (a *application) scheduler(ctx context.Context) {
	for {
		cfg := a.currentConfig()
		minHours, maxHours, ok := intervalRange(cfg)
		if !ok {
			select {
			case <-a.schedulerWake:
				continue
			case <-ctx.Done():
				return
			}
		}
		interval := max(time.Duration((minHours+mathrand.Float64()*(maxHours-minHours))*float64(time.Hour)), time.Minute)
		slog.Info("next scheduled check", "interval", interval.Round(time.Minute).String(), "at", time.Now().Add(interval).Format("15:04"))
		timer := time.NewTimer(interval)
		select {
		case <-timer.C:
			if _, err := a.checkWithOptions(ctx, checkOptions{Kind: "scheduled", SaveLatest: true}); err != nil {
				if errors.Is(err, web.ErrCheckAlreadyRunning) {
					slog.Warn("scheduled check skipped", "err", err)
					continue
				}
				slog.Error("scheduled check failed", "err", err)
			}
		case <-a.schedulerWake:
			if !timer.Stop() {
				<-timer.C
			}
			continue
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return
		}
	}
}

func filterProvider(cfg config.Config, providerID string) (config.Config, bool) {
	for _, provider := range cfg.Providers {
		if provider.ID == providerID {
			cfg.Providers = []config.ProviderConfig{provider}
			return cfg, true
		}
	}
	return cfg, false
}

func mergeNotificationSecrets(settings *config.RuntimeSettings, current config.Config) {
	if settings.ClearNotifyWebhookURL {
		settings.NotifyWebhookURL = ""
	} else if settings.NotifyWebhookURL == "" {
		settings.NotifyWebhookURL = current.NotifyWebhookURL
	}
	if settings.ClearNotifyTelegramBotToken {
		settings.NotifyTelegramBotToken = ""
	} else if settings.NotifyTelegramBotToken == "" {
		settings.NotifyTelegramBotToken = current.NotifyTelegramBotToken
	}
	if settings.ClearNotifyTelegramChatID {
		settings.NotifyTelegramChatID = ""
	} else if settings.NotifyTelegramChatID == "" {
		settings.NotifyTelegramChatID = current.NotifyTelegramChatID
	}
	settings.NotifyWebhookURLSet = settings.NotifyWebhookURL != ""
	settings.NotifyTelegramBotTokenSet = settings.NotifyTelegramBotToken != ""
	settings.NotifyTelegramChatIDSet = settings.NotifyTelegramChatID != ""
	settings.ClearNotifyWebhookURL = false
	settings.ClearNotifyTelegramBotToken = false
	settings.ClearNotifyTelegramChatID = false
}

func intervalRange(cfg config.Config) (float64, float64, bool) {
	minHours := cfg.AutoCheckIntervalMinHours
	maxHours := cfg.AutoCheckIntervalMaxHours
	if minHours <= 0 && maxHours <= 0 {
		return 0, 0, false
	}
	if minHours <= 0 {
		minHours = maxHours
	}
	if maxHours <= 0 {
		maxHours = minHours
	}
	if maxHours < minHours {
		minHours, maxHours = maxHours, minHours
	}
	return minHours, maxHours, maxHours > 0
}

func generateAdminToken() (string, error) {
	buf := make([]byte, 18)
	if _, err := crand.Read(buf); err != nil {
		return "", fmt.Errorf("generate admin token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func healthcheck(port int) error {
	client := &http.Client{Timeout: 3 * time.Second}
	response, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/health", port))
	if err != nil {
		return fmt.Errorf("healthcheck failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("healthcheck returned %s", response.Status)
	}
	return nil
}
