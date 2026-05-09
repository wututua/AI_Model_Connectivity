package main

import (
	"context"
	"errors"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"cg/internal/config"
	"cg/internal/probe"
	"cg/internal/report"
	"cg/internal/storage"
	"cg/internal/web"
)

func main() {
	cfg, err := config.Load(".env")
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	store := storage.New(cfg.DataDir, cfg.WebDir)
	if err := web.EnsureAssets(cfg.WebDir); err != nil {
		log.Fatalf("write web assets: %v", err)
	}

	broker := web.NewBroker()
	app := &application{cfg: cfg, store: store, broker: broker}
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
			if _, err := app.check(ctx); err != nil {
				if errors.Is(err, web.ErrCheckAlreadyRunning) {
					log.Printf("startup check skipped: %v", err)
					return
				}
				log.Printf("startup check failed: %v", err)
			}
		}()
	}
	go app.scheduler(ctx)

	server := web.NewServer(cfg, store, app.check, broker).HTTPServer()
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
	cfg     config.Config
	store   storage.JSONStore
	broker  *web.Broker
	mu      sync.Mutex
	running bool
}

func (a *application) check(ctx context.Context) (report.Report, error) {
	a.mu.Lock()
	if a.running {
		a.mu.Unlock()
		return report.Report{}, web.ErrCheckAlreadyRunning
	}
	a.running = true
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		a.running = false
		a.mu.Unlock()
	}()
	return a.runCheck(ctx)
}

func (a *application) runCheck(ctx context.Context) (report.Report, error) {
	started := time.Now()
	runner := probe.NewRunner(a.cfg)
	results, providerErrors, err := runner.Run(ctx)
	if err != nil && len(results) == 0 {
		return report.Report{}, err
	}
	history := map[string][]report.HistoryRecord{}
	if a.cfg.EnableHistory {
		loaded, loadErr := storage.ReadJSON[map[string][]report.HistoryRecord](a.store.HistoryPath(), map[string][]report.HistoryRecord{})
		if loadErr != nil {
			log.Printf("load history failed: %v", loadErr)
		} else {
			history = loaded
		}
	}
	value, updatedHistory := report.Build(a.cfg, results, providerErrors, history, started)
	if a.cfg.EnableHistory {
		if err := storage.WriteJSON(a.store.HistoryPath(), updatedHistory); err != nil {
			log.Printf("save history failed: %v", err)
		}
	}
	if err := storage.WriteJSON(a.store.LatestReportPath(), value); err != nil {
		return report.Report{}, err
	}
	if a.broker != nil {
		a.broker.Publish(value)
	}
	log.Printf("check finished: ok=%d slow=%d error=%d total=%d", value.OKCount, value.SlowCount, value.ErrorCount, value.Total)
	return value, nil
}

func (a *application) scheduler(ctx context.Context) {
	for {
		minHours, maxHours, ok := intervalRange(a.cfg)
		if !ok {
			return
		}
		interval := time.Duration((minHours + rand.Float64()*(maxHours-minHours)) * float64(time.Hour))
		if interval < time.Minute {
			interval = time.Minute
		}
		select {
		case <-time.After(interval):
			if _, err := a.check(ctx); err != nil {
				if errors.Is(err, web.ErrCheckAlreadyRunning) {
					log.Printf("scheduled check skipped: %v", err)
					continue
				}
				log.Printf("scheduled check failed: %v", err)
			}
		case <-ctx.Done():
			return
		}
	}
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
