package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"cg/internal/config"
	"cg/internal/notify"
	"cg/internal/probe"
	"cg/internal/report"
	"cg/internal/storage"
	"cg/internal/web"
)

// Version is set at build time via -ldflags "-X cg/cmd/cg.Version=v1.0.4"
var Version = "dev"

func main() {
	log.Printf("AI_Model_Connectivity version: %s", Version)
	baseCfg, err := config.Load(".env")
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	store, err := storage.NewSQLite(context.Background(), baseCfg.DatabasePath, baseCfg.DataDir)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer store.Close()
	if err := web.EnsureAssets(baseCfg.WebDir); err != nil {
		log.Fatalf("write web assets: %v", err)
	}

	runtimeCfg, ok, err := store.LoadRuntimeConfig(context.Background())
	if err != nil {
		log.Fatalf("load runtime config: %v", err)
	}
	cfg := baseCfg
	if ok {
		cfg = config.ApplyRuntimeConfig(baseCfg, runtimeCfg)
	} else {
		runtimeCfg = config.RuntimeConfigFromConfig(baseCfg)
		if err := store.SaveRuntimeConfig(context.Background(), runtimeCfg); err != nil {
			log.Fatalf("save runtime config: %v", err)
		}
	}

	broker := web.NewBroker()
	app := &application{baseCfg: baseCfg, cfg: cfg, store: store, broker: broker, schedulerWake: make(chan struct{}, 1)}
	args := os.Args[1:]
	if len(args) > 0 {
		switch args[0] {
		case "check", "once":
			if _, err := app.check(context.Background()); err != nil {
				log.Fatalf("check failed: %v", err)
			}
			return
		case "serve":
			// fall through
		default:
			log.Fatalf("unknown command %q; use serve or check", args[0])
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if cfg.AutoCheckRunOnStart {
		go func() {
			if _, err := app.checkWithOptions(ctx, checkOptions{Kind: "startup", SaveLatest: true}); err != nil {
				if errors.Is(err, web.ErrCheckAlreadyRunning) {
					log.Printf("startup check skipped: %v", err)
					return
				}
				log.Printf("startup check failed: %v", err)
			}
		}()
	}
	go app.scheduler(ctx)

	server := web.NewServer(cfg, store, app.check, broker, app).HTTPServer()
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- server.ListenAndServe()
	}()

	select {
	case err := <-serverErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Fatalf("shutdown server: %v", err)
		}
		if err := <-serverErr; err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}
}

type application struct {
	baseCfg        config.Config
	cfg            config.Config
	store          *storage.SQLiteStore
	broker         *web.Broker
	schedulerWake  chan struct{}
	mu             sync.Mutex
	running        bool
	runCancel      context.CancelFunc
	taskID         int64
	taskKind       string
	taskProviderID string
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
	return a.checkWithOptions(ctx, checkOptions{Kind: "provider", ProviderID: providerID, SaveLatest: false})
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
	a.mu.Lock()
	defer a.mu.Unlock()
	cfg := a.cfg
	return web.RunningState{
		Running:                   a.running,
		TaskID:                    a.taskID,
		Kind:                      a.taskKind,
		ProviderID:                a.taskProviderID,
		AutoCheckIntervalMinHours: cfg.AutoCheckIntervalMinHours,
		AutoCheckIntervalMaxHours: cfg.AutoCheckIntervalMaxHours,
	}
}

func (a *application) AdminConfig(context.Context) (config.AdminConfig, error) {
	return config.AdminConfigFromConfig(a.currentConfig()), nil
}

func (a *application) UpdateSettings(ctx context.Context, settings config.RuntimeSettings) (config.AdminConfig, error) {
	if err := config.ValidateRuntimeSettings(settings); err != nil {
		return config.AdminConfig{}, err
	}
	current := a.currentConfig()
	runtimeCfg := config.RuntimeConfig{Settings: settings, Providers: current.Providers}
	if err := a.replaceRuntimeConfig(ctx, runtimeCfg); err != nil {
		return config.AdminConfig{}, err
	}
	return config.AdminConfigFromConfig(a.currentConfig()), nil
}

func (a *application) UpsertProvider(ctx context.Context, id string, update config.ProviderUpdate) (config.SafeProviderConfig, error) {
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
	return config.ConfigExport{Settings: config.SettingsFromConfig(current), Providers: config.SafeProviders(current.Providers)}, nil
}

func (a *application) ImportConfig(ctx context.Context, value config.ConfigImport) (config.AdminConfig, error) {
	if err := config.ValidateRuntimeSettings(value.Settings); err != nil {
		return config.AdminConfig{}, err
	}
	current := a.currentConfig()
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

func (a *application) ListTasks(ctx context.Context, query storage.TaskQuery) ([]storage.CheckTask, error) {
	return a.store.ListCheckTasks(ctx, query)
}

func (a *application) GetTask(ctx context.Context, id int64) (storage.CheckTask, error) {
	return a.store.GetCheckTask(ctx, id)
}

func (a *application) currentConfig() config.Config {
	a.mu.Lock()
	defer a.mu.Unlock()
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
		log.Printf("finish check task failed: %v", err)
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
	if err != nil && len(results) == 0 {
		return report.Report{}, err
	}
	history := map[string][]report.HistoryRecord{}
	if cfg.EnableHistory {
		loaded, loadErr := a.store.LoadHistory(ctx, max(cfg.HistorySize, cfg.MaxHistoryRecords), cfg.StatsWindowDays)
		if loadErr != nil {
			log.Printf("load history failed: %v", loadErr)
		} else {
			history = loaded
		}
	}
	value, _ := report.Build(cfg, results, providerErrors, history, started)
	if cfg.EnableHistory {
		if err := a.store.AppendResults(ctx, results, time.Now()); err != nil {
			log.Printf("save history failed: %v", err)
		}
	}
	if options.SaveLatest {
		if err := a.store.SaveLatestReport(ctx, value); err != nil {
			return report.Report{}, err
		}
		if a.broker != nil {
			a.broker.Publish(value)
		}
		if err := notify.New(cfg, storage.SQLiteNotifyStateStore{Store: a.store}).SendIfNeeded(ctx, value); err != nil {
			log.Printf("send notify failed: %v", err)
		}
	}
	log.Printf("check finished: ok=%d slow=%d error=%d total=%d", value.OKCount, value.SlowCount, value.ErrorCount, value.Total)
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
		interval := time.Duration((minHours + rand.Float64()*(maxHours-minHours)) * float64(time.Hour))
		if interval < time.Minute {
			interval = time.Minute
		}
		timer := time.NewTimer(interval)
		select {
		case <-timer.C:
			if _, err := a.checkWithOptions(ctx, checkOptions{Kind: "scheduled", SaveLatest: true}); err != nil {
				if errors.Is(err, web.ErrCheckAlreadyRunning) {
					log.Printf("scheduled check skipped: %v", err)
					continue
				}
				log.Printf("scheduled check failed: %v", err)
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

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
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