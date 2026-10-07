package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	mathrand "math/rand"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"cg/internal/config"
	"cg/internal/metrics"
	"cg/internal/probe"
	"cg/internal/provider"
	"cg/internal/report"
	"cg/internal/storage"
	"cg/internal/update"
	"cg/internal/web"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	args := os.Args[1:]
	if len(args) == 1 && args[0] == "--update-protocol" {
		fmt.Println("1")
		return
	}
	if len(args) == 1 && args[0] == "update-worker" {
		if err := update.RunWorker(); err != nil {
			slog.Error("system update failed", "err", err)
			os.Exit(1)
		}
		return
	}
	if len(args) == 1 && (args[0] == "--version" || args[0] == "version") {
		fmt.Println(versionString())
		return
	}

	baseCfg, err := config.Load()
	if err != nil {
		slog.Error("load config", "err", err)
		os.Exit(1)
	}
	if len(args) > 0 && args[0] == "healthcheck" {
		if err := healthcheck(baseCfg.AppPort); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	store, err := storage.NewSQLite(context.Background(), baseCfg.DatabasePath, baseCfg.DataDir)
	if err != nil {
		slog.Error("open database", "err", err)
		os.Exit(1)
	}
	defer store.Close()
	if len(args) > 0 && args[0] == "recover-admin" {
		if len(args) != 2 {
			fmt.Fprintln(os.Stderr, "usage: model-connectivity recover-admin <existing-admin-username> (stop the service first)")
			os.Exit(1)
		}
		password, err := recoverAdmin(context.Background(), store, args[1])
		if err != nil {
			slog.Error("recover administrator", "err", err)
			os.Exit(1)
		}
		fmt.Println("Temporary administrator password:", password)
		fmt.Println("Old sessions revoked. Change this password on first login. Keep this output private.")
		return
	}
	if err := store.RecoverInterruptedTasks(context.Background()); err != nil {
		slog.Error("recover interrupted tasks", "err", err)
		os.Exit(1)
	}
	if err := store.RecoverInterruptedNotifications(context.Background()); err != nil {
		slog.Error("recover interrupted notifications", "err", err)
		os.Exit(1)
	}
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
	cfg.Providers = config.ReconcileProviderRevisions(cfg.Providers, cfg.Providers)
	if err := store.SaveRuntimeConfig(context.Background(), config.RuntimeConfigFromConfig(cfg)); err != nil {
		slog.Error("save provider revisions", "err", err)
		os.Exit(1)
	}
	app := &application{baseCfg: baseCfg, cfg: cfg, store: store, broker: broker, metrics: metrics.New(), schedulerWake: make(chan struct{}, 1)}
	app.updater = update.New(version, commit)

	if err := app.initializeUsers(context.Background()); err != nil {
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
			slog.Error("unknown command", "cmd", args[0], "hint", "use serve, check, healthcheck, --version or recover-admin")
			os.Exit(1)
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var background sync.WaitGroup
	if cfg.AutoCheckRunOnStart {
		background.Add(1)
		go func() {
			defer background.Done()
			if _, err := app.checkWithOptions(ctx, checkOptions{Kind: "startup", SaveLatest: true}); err != nil {
				if errors.Is(err, web.ErrCheckAlreadyRunning) {
					slog.Warn("startup check skipped", "err", err)
					return
				}
				slog.Error("startup check failed", "err", err)
			}
		}()
	}
	background.Add(1)
	go func() {
		defer background.Done()
		app.scheduler(ctx)
	}()
	background.Add(1)
	go func() { defer background.Done(); app.backupScheduler(ctx) }()

	srv := web.NewServer(cfg, store, app.check, broker, app)
	srv.SetMetrics(app.metrics)
	srv.SetUpdater(app)
	server := srv.HTTPServer()
	server.BaseContext = func(net.Listener) context.Context { return ctx }
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
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := app.shutdown(shutdownCtx); err != nil {
			slog.Error("wait for active check", "err", err)
		}
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
		backgroundDone := make(chan struct{})
		go func() {
			background.Wait()
			close(backgroundDone)
		}()
		select {
		case <-backgroundDone:
		case <-shutdownCtx.Done():
			slog.Warn("background shutdown timed out")
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
	notificationMu sync.Mutex
	running        bool
	runCancel      context.CancelFunc
	runDone        chan struct{}
	shuttingDown   bool
	updater        *update.Manager
	taskID         int64
	taskKind       string
	taskProviderID string
	runStarted     time.Time
	progress       probe.Progress
}

type checkOptions struct {
	Kind       string
	ProviderID string
	SaveLatest bool
	Targets    []config.ModelTarget
}

type checkRun struct {
	ctx     context.Context
	cancel  context.CancelFunc
	started time.Time
	task    storage.CheckTask
}

func (a *application) StartCheck(ctx context.Context, providerID string) (storage.CheckTask, error) {
	if err := ctx.Err(); err != nil {
		return storage.CheckTask{}, err
	}
	options := checkOptions{Kind: "manual", ProviderID: providerID, SaveLatest: true}
	if providerID != "" {
		cfg, ok := filterProvider(a.currentConfig(), providerID)
		if !ok || !cfg.Providers[0].Enabled || !cfg.Providers[0].ProbeEnabled {
			return storage.CheckTask{}, web.ErrProviderUnavailable
		}
		options.Kind = "provider"
	}
	// Acceptance is synchronous; execution belongs to the service, not the HTTP connection.
	background, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	run, err := a.beginCheck(background, options)
	if err != nil {
		cancel()
		return storage.CheckTask{}, err
	}
	go func() {
		defer cancel()
		if _, err := a.executeCheck(run, options); err != nil && !errors.Is(err, context.Canceled) {
			slog.Error("background check failed", "task_id", run.task.ID, "err", err)
		}
	}()
	return run.task, nil
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

func (a *application) shutdown(ctx context.Context) error {
	a.mu.Lock()
	a.shuttingDown = true
	done := a.runDone
	if a.runCancel != nil {
		a.runCancel()
	}
	a.mu.Unlock()
	if done == nil {
		return nil
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (a *application) RunningState() web.RunningState {
	a.mu.RLock()
	defer a.mu.RUnlock()
	cfg := a.cfg
	progress := a.progress
	progress.Active = append([]config.ModelTarget{}, progress.Active...)
	elapsed := int64(0)
	if a.running {
		elapsed = time.Since(a.runStarted).Milliseconds()
	} else {
		progress = probe.Progress{Phase: "idle", Active: []config.ModelTarget{}}
	}
	return web.RunningState{
		Progress:                  progress,
		ElapsedMS:                 elapsed,
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
		if id == "" && strings.EqualFold(provider.ID, strings.TrimSpace(update.ID)) {
			return config.SafeProviderConfig{}, config.ErrProviderExists
		}
		if id != "" && provider.ID == id {
			found = i
			break
		}
	}
	if id != "" && found < 0 {
		return config.SafeProviderConfig{}, config.ErrProviderNotFound
	}
	existing := config.ProviderConfig{Enabled: true}
	if found >= 0 {
		existing = providers[found]
	}
	provider, err := config.ApplyProviderUpdate(existing, update)
	if err != nil {
		return config.SafeProviderConfig{}, err
	}
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
		return fmt.Errorf("%w: %q", config.ErrProviderNotFound, id)
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
		existing[strings.ToLower(provider.ID)] = provider
	}
	providers := make([]config.ProviderConfig, 0, len(value.Providers))
	for _, item := range value.Providers {
		provider, err := config.ApplyProviderUpdate(existing[strings.ToLower(strings.TrimSpace(item.ID))], item)
		if err != nil {
			return config.AdminConfig{}, err
		}
		providers = append(providers, provider)
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
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.cfg
}

func (a *application) replaceRuntimeConfig(ctx context.Context, value config.RuntimeConfig) error {
	value.Providers = config.ReconcileProviderRevisions(a.currentConfig().Providers, value.Providers)
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
	a.publishLatest(ctx)
	return nil
}

func (a *application) publishLatest(ctx context.Context) {
	value, err := a.store.LatestReport(ctx)
	if err != nil {
		slog.Warn("load report for settings update", "err", err)
		return
	}
	value = report.WithConfig(value, config.AdminConfigFromConfig(a.currentConfig()))
	if value.GeneratedAt != "" {
		if err := a.store.SaveLatestReport(ctx, value); err != nil {
			slog.Warn("save updated status snapshot", "err", err)
		}
	}
	if a.broker != nil {
		a.broker.Publish(value)
	}
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
	run, err := a.beginCheck(ctx, options)
	if err != nil {
		return report.Report{}, err
	}
	return a.executeCheck(run, options)
}

func (a *application) beginCheck(ctx context.Context, options checkOptions) (checkRun, error) {
	budget, err := a.store.RequestBudget(ctx, a.currentConfig().DailyRequestLimit)
	if err != nil {
		return checkRun{}, err
	}
	if budget.Exhausted {
		return checkRun{}, storage.ErrBudgetExceeded
	}
	run := checkRun{started: time.Now()}
	runCtx, cancel := context.WithCancel(ctx)
	run.ctx, run.cancel = runCtx, cancel

	a.mu.Lock()
	if a.shuttingDown || a.updater != nil && a.updater.Busy() {
		a.mu.Unlock()
		cancel()
		return checkRun{}, web.ErrShuttingDown
	}
	if a.running {
		a.mu.Unlock()
		cancel()
		return checkRun{}, web.ErrCheckAlreadyRunning
	}
	a.running = true
	a.runCancel = cancel
	a.runDone = make(chan struct{})
	a.taskKind = options.Kind
	a.taskProviderID = options.ProviderID
	a.runStarted = run.started
	a.progress = probe.Progress{Phase: "discovering", Active: []config.ModelTarget{}}
	a.mu.Unlock()
	run.task = storage.CheckTask{Kind: options.Kind, Status: "running", ProviderID: options.ProviderID, StartedAt: run.started.UTC().Format(time.RFC3339)}
	taskID, err := a.store.CreateCheckTask(runCtx, run.task)
	if err != nil {
		cancel()
		a.finishRunState()
		return checkRun{}, err
	}
	run.task.ID = taskID
	a.mu.Lock()
	a.taskID = taskID
	a.mu.Unlock()
	return run, nil
}

func (a *application) executeCheck(run checkRun, options checkOptions) (report.Report, error) {
	defer a.finishRunState()
	defer run.cancel()
	update := storage.CheckTaskUpdate{}
	value, runErr := a.runCheck(run.ctx, options, &update)
	finished := time.Now()
	status := "success"
	errorMessage := ""
	if runErr != nil {
		if errors.Is(run.ctx.Err(), context.Canceled) {
			status = "canceled"
		} else {
			status = "error"
		}
		errorMessage = runErr.Error()
	}
	update.Status, update.FinishedAt = status, finished
	update.ElapsedMS = int(finished.Sub(run.started).Milliseconds())
	update.ErrorMessage, update.ReportGeneratedAt = errorMessage, value.GeneratedAt
	finishCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := a.store.FinishCheckTask(finishCtx, run.task.ID, update); err != nil {
		slog.Error("finish check task failed", "err", err)
	}
	if a.metrics != nil {
		a.metrics.RecordCheck(options.Kind, status, finished.Sub(run.started).Seconds())
	}
	return value, runErr
}

func (a *application) finishRunState() {
	a.mu.Lock()
	a.running = false
	a.runCancel = nil
	a.taskID = 0
	a.taskKind = ""
	a.taskProviderID = ""
	a.progress = probe.Progress{Phase: "idle", Active: []config.ModelTarget{}}
	if a.runDone != nil {
		close(a.runDone)
		a.runDone = nil
	}
	a.mu.Unlock()
}

func (a *application) runCheck(ctx context.Context, options checkOptions, counts *storage.CheckTaskUpdate) (value report.Report, runErr error) {
	started := time.Now()
	cfg := a.currentConfig()
	fullCfg := cfg
	monitoring, err := a.store.MonitoringSettings(ctx)
	if err != nil {
		return report.Report{}, err
	}
	if options.ProviderID != "" {
		filtered, ok := filterProvider(cfg, options.ProviderID)
		if !ok {
			return report.Report{}, fmt.Errorf("provider %q not found", options.ProviderID)
		}
		cfg = filtered
	}
	if len(options.Targets) > 0 {
		previous, err := a.store.LatestReport(ctx)
		if err != nil {
			return report.Report{}, err
		}
		allowed := allowedModelTargets(cfg, report.WithConfig(previous, config.AdminConfigFromConfig(cfg)))
		for _, target := range options.Targets {
			if !allowed[provider.ModelKey(target.ProviderID, target.Model)] {
				return report.Report{}, fmt.Errorf("%w: 配置已变更，请重新选择模型", web.ErrInvalidSelection)
			}
		}
		cfg = selectedConfig(cfg, options.Targets)
	}
	runner := probe.NewRunner(cfg)
	runner.SetSlowThresholds(monitoring.Schedules)
	runner.SetCatalogObserver(func(ctx context.Context, observed config.ProviderConfig, models []string) ([]string, error) {
		a.configMu.Lock()
		defer a.configMu.Unlock()
		for _, current := range a.currentConfig().Providers {
			if current.ID == observed.ID && current.ConnectionRevision == observed.ConnectionRevision && len(current.Models) == 0 {
				return a.store.ObserveCatalog(ctx, observed, models)
			}
		}
		return nil, errors.New("provider changed during discovery")
	})
	runner.SetRequestGuard(a.reserveRequest)
	runner.SetObserver(func(progress probe.Progress) {
		a.mu.Lock()
		a.progress = progress
		a.mu.Unlock()
	})
	results, providerErrors, err := runner.Run(ctx)
	for _, result := range results {
		if !result.Completed {
			continue
		}
		switch result.Status {
		case "ok":
			counts.OKCount++
		case "slow":
			counts.SlowCount++
		case "error":
			counts.ErrorCount++
		}
		if a.metrics != nil {
			a.metrics.RecordProbe(result)
		}
	}
	counts.Total = counts.OKCount + counts.SlowCount + counts.ErrorCount
	usageSaved := false
	defer func() {
		if usageSaved {
			return
		}
		completed := make([]probe.Result, 0, len(results))
		for _, result := range results {
			if result.Completed {
				completed = append(completed, result)
			}
		}
		if len(completed) == 0 {
			return
		}
		// An interrupted batch must not replace the report or history, but usage is real.
		saveCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := a.store.RecordResults(saveCtx, completed, time.Now(), cfg.MaxHistoryRecords, false); err != nil {
			runErr = errors.Join(runErr, fmt.Errorf("save completed probe usage: %w", err))
		}
	}()
	if err != nil {
		return report.Report{}, err
	}
	var previous report.Report
	if len(providerErrors) > 0 || options.ProviderID != "" && options.SaveLatest || len(options.Targets) > 0 {
		previous, err = a.store.LatestReport(ctx)
		if err != nil {
			return report.Report{}, fmt.Errorf("load latest report: %w", err)
		}
		previous = report.WithConfig(previous, config.AdminConfigFromConfig(fullCfg))
		results = report.WithDiscoveryGaps(cfg, results, providerErrors, previous)
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
	value, _ = report.Build(cfg, results, providerErrors, history, started)
	if len(options.Targets) > 0 {
		value = report.MergeModels(previous, value)
	} else if options.ProviderID != "" && options.SaveLatest {
		value = report.MergeProvider(previous, value, options.ProviderID)
	}
	observed := value
	a.setPhase("saving")
	a.configMu.Lock()
	value = report.WithConfig(value, config.AdminConfigFromConfig(a.currentConfig()))
	var latest *report.Report
	if options.SaveLatest {
		latest = &value
	}
	if err := a.store.RecordCheck(ctx, results, time.Now(), cfg.MaxHistoryRecords, cfg.EnableHistory, latest); err != nil {
		a.configMu.Unlock()
		return report.Report{}, fmt.Errorf("save probe results: %w", err)
	}
	usageSaved = true
	currentProviders := a.currentConfig()
	validResults := []probe.Result{}
	for _, result := range results {
		for _, p := range cfg.Providers {
			if p.ID != result.ProviderID {
				continue
			}
			for _, current := range currentProviders.Providers {
				if p.ID == current.ID && p.ConnectionRevision == current.ConnectionRevision {
					validResults = append(validResults, result)
				}
			}
		}
	}
	if len(options.Targets) == 0 {
		discoveryFailed := map[string]bool{}
		for _, failure := range providerErrors {
			discoveryFailed[failure.ProviderID] = true
		}
		for _, observedProvider := range cfg.Providers {
			if !observedProvider.Enabled || !observedProvider.ProbeEnabled {
				continue
			}
			for _, current := range currentProviders.Providers {
				if current.ID != observedProvider.ID || current.ConnectionRevision != observedProvider.ConnectionRevision {
					continue
				}
				status := "ok"
				if discoveryFailed[current.ID] {
					status = "error"
				}
				validResults = append(validResults, probe.Result{ProviderID: current.ID, Model: "", Status: status, Completed: true, CheckedAt: time.Now().UTC().Format(time.RFC3339)})
			}
		}
	}
	if err := a.store.ObserveIncidents(ctx, cfg, validResults); err != nil {
		slog.Warn("save incidents failed", "err", err)
	}
	if options.SaveLatest && a.broker != nil {
		a.broker.Publish(value)
	}
	a.configMu.Unlock()
	if options.SaveLatest && len(options.Targets) == 0 {
		a.sendRuleAlerts(ctx, observed, fullCfg, options.ProviderID, monitoring)
	}
	if options.SaveLatest && len(options.Targets) == 0 && options.ProviderID == "" {
		a.setPhase("notifying")
		notifyCfg := a.currentConfig()
		if err := a.notificationClient(notifyCfg).SendCheckIfNeeded(ctx, report.WithConfig(observed, config.AdminConfigFromConfig(notifyCfg)), fullCfg); err != nil {
			slog.Warn("send notify failed", "err", err)
		}
	}
	slog.Info("check finished", "ok", value.OKCount, "slow", value.SlowCount, "error", value.ErrorCount, "total", value.Total)
	return value, nil
}

func (a *application) scheduler(ctx context.Context) {
	nextGlobal := time.Time{}
	var previousRange [2]float64
	for {
		cfg := a.currentConfig()
		minHours, maxHours, ok := intervalRange(cfg)
		settings, err := a.store.MonitoringSettings(ctx)
		independent := false
		for _, schedule := range settings.Schedules {
			if schedule.IntervalMinutes <= 0 {
				continue
			}
			for _, p := range cfg.Providers {
				if p.ID == schedule.ProviderID && p.Enabled && p.ProbeEnabled {
					independent = true
				}
			}
		}
		if err == nil && independent {
			nextGlobal = time.Time{}
			a.runProviderSchedules(ctx, cfg, settings)
		} else if err == nil && ok {
			currentRange := [2]float64{minHours, maxHours}
			if nextGlobal.IsZero() || previousRange != currentRange {
				interval := max(time.Duration((minHours+mathrand.Float64()*(maxHours-minHours))*float64(time.Hour)), time.Minute)
				nextGlobal, previousRange = time.Now().Add(interval), currentRange
			}
			if !time.Now().Before(nextGlobal) {
				if _, err := a.checkWithOptions(ctx, checkOptions{Kind: "scheduled", SaveLatest: true}); err != nil && !errors.Is(err, web.ErrCheckAlreadyRunning) && !errors.Is(err, storage.ErrBudgetExceeded) {
					slog.Warn("scheduled check failed", "err", err)
				}
				nextGlobal = time.Time{}
			}
		} else if !ok {
			nextGlobal = time.Time{}
		}
		timer := time.NewTimer(time.Minute)
		select {
		case <-timer.C:
		case <-a.schedulerWake:
			timer.Stop()
		case <-ctx.Done():
			timer.Stop()
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
