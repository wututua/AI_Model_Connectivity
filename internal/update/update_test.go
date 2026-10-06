package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func fixtureRelease(version string, preview bool, published string) map[string]any {
	return map[string]any{"tag_name": version, "draft": false, "prerelease": preview, "published_at": published,
		"body":   "## 中文\n更新说明\n## English\nRelease notes",
		"assets": []map[string]string{{"name": assetName("linux/amd64"), "state": "uploaded"}, {"name": "SHA256SUMS.txt", "state": "uploaded"}}}
}

func testManager(t *testing.T, handler http.HandlerFunc) *Manager {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	m := New("v1.0.0-beta.2", "fixture")
	m.api, m.client, m.platform = server.URL, server.Client(), "linux/amd64"
	m.dir = t.TempDir()
	m.supported = func() bool { return true }
	if err := os.Mkdir(filepath.Join(m.dir, "requests"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Status(); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestCheckChannelsCacheAndVersionOrdering(t *testing.T) {
	var calls atomic.Int32
	m := testManager(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path == "/releases/latest" {
			w.WriteHeader(404)
			return
		}
		if r.URL.Query().Get("per_page") != "100" {
			t.Error("preview page size changed")
		}
		_ = json.NewEncoder(w).Encode([]any{
			fixtureRelease("v1.0.0-rc.9", true, "2026-10-01T00:00:00Z"),
			fixtureRelease("v1.0.0-rc.10", true, "2026-10-02T00:00:00Z"),
			fixtureRelease("../../bad", true, "2026-10-03T00:00:00Z"),
		})
	})
	stable, err := m.Check(context.Background(), "stable")
	if err != nil || stable.Release != nil || stable.Available {
		t.Fatal(stable, err)
	}
	preview, err := m.Check(context.Background(), "preview")
	if err != nil || preview.Release == nil || preview.Release.Version != "v1.0.0-rc.10" || !preview.Available {
		t.Fatal(preview, err)
	}
	if preview.Release.URL != "https://github.com/"+Repository+"/releases/tag/v1.0.0-rc.10" {
		t.Fatal("untrusted URL")
	}
	_, _ = m.Check(context.Background(), "preview")
	if calls.Load() != 2 {
		t.Fatal("cache not used")
	}
	m.version = "v1.0.0"
	m.cache = map[string]Check{}
	result, _ := m.Check(context.Background(), "preview")
	if result.Available {
		t.Fatal("automatic downgrade offered")
	}
}

func TestStableRejectsPrereleasesAndBadResponses(t *testing.T) {
	for _, code := range []int{403, 429, 500} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			m := testManager(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(code) })
			if _, err := m.Check(context.Background(), "stable"); err == nil {
				t.Fatal("HTTP error accepted")
			}
		})
	}
	m := testManager(t, func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(fixtureRelease("v1.0.0-rc.1", false, "2026-10-02T00:00:00Z"))
	})
	result, err := m.Check(context.Background(), "stable")
	if err != nil || result.Release != nil {
		t.Fatal("prerelease accepted on stable", err)
	}
	if _, err := m.Check(context.Background(), "nightly"); !errors.Is(err, ErrChannel) {
		t.Fatal(err)
	}
}

func TestStartRequiresNewMatchingVersionAndSingleJob(t *testing.T) {
	m := testManager(t, func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(fixtureRelease("v1.0.0", false, "2026-10-02T00:00:00Z"))
	})
	requestID := m.requestID
	for _, version := range []string{"v0.1.0", "v1.0.0;touch /tmp/unsafe", "../../unsafe", "v1.0.0-rc.1"} {
		if _, err := m.Start(context.Background(), "stable", version, requestID); !errors.Is(err, ErrVersion) {
			t.Fatal(version, err)
		}
	}
	var accepted atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := m.Start(context.Background(), "stable", "v1.0.0", requestID)
			if err == nil {
				accepted.Add(1)
			} else if !errors.Is(err, ErrRequest) {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if accepted.Load() != 1 {
		t.Fatal("multiple requests queued", accepted.Load())
	}
	job, err := m.readJob()
	if err != nil || job == nil || job.Status != "pending" || !idPattern.MatchString(job.ID) {
		t.Fatal(job, err)
	}
	m.supported = func() bool { return false }
	if _, err := m.Start(context.Background(), "stable", "v1.0.0", requestID); !errors.Is(err, ErrUnsupported) {
		t.Fatal(err)
	}
}

func TestStartRejectsMissingAssetsAndDevelopmentBuild(t *testing.T) {
	m := testManager(t, func(w http.ResponseWriter, _ *http.Request) {
		value := fixtureRelease("v1.0.0", false, "2026-10-02T00:00:00Z")
		value["assets"] = []any{}
		_ = json.NewEncoder(w).Encode(value)
	})
	if _, err := m.Start(context.Background(), "stable", "v1.0.0", m.requestID); !errors.Is(err, ErrVersion) {
		t.Fatal(err)
	}
	m.version = "dev"
	m.cache = map[string]Check{}
	value, err := m.Check(context.Background(), "stable")
	if err != nil || value.Available {
		t.Fatal(value, err)
	}
}

func TestReadJobRechecksStatusAfterWorkerConsumesRequest(t *testing.T) {
	for _, previous := range []string{"", "succeeded", "failed", "rolled_back"} {
		t.Run(previous, func(t *testing.T) {
			statusReads := 0
			job, err := readJobFiles("fixture", func(path string, value any, _ int64) error {
				if filepath.Base(path) == "request.json" {
					return os.ErrNotExist
				}
				statusReads++
				if statusReads == 1 {
					if previous == "" {
						return os.ErrNotExist
					}
					*value.(*Job) = Job{ID: "old", Status: previous}
				} else {
					*value.(*Job) = Job{ID: "current", Status: "running"}
				}
				return nil
			})
			if err != nil || job == nil || job.ID != "current" || !job.Active() || statusReads != 2 {
				t.Fatalf("missed worker handoff: job=%+v reads=%d err=%v", job, statusReads, err)
			}
		})
	}
}

func TestReadJobRereadFailsClosedAndKeepsRecoveryState(t *testing.T) {
	reads := 0
	_, err := readJobFiles("fixture", func(_ string, _ any, _ int64) error {
		reads++
		if reads < 3 {
			return os.ErrNotExist
		}
		return os.ErrPermission
	})
	if !errors.Is(err, os.ErrPermission) {
		t.Fatal("reread failure treated as idle", err)
	}
	job, err := readJobFiles("fixture", func(path string, value any, _ int64) error {
		if filepath.Base(path) != "status.json" {
			t.Fatal("unresolved recovery must take precedence over the inbox")
		}
		*value.(*Job) = Job{ID: "unresolved", Status: "recovery_required"}
		return nil
	})
	if err != nil || job == nil || !job.Active() {
		t.Fatal(job, err)
	}
}

func TestResolveInvalidatesDelayedSubmissionAndAllowsNewConfirmation(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	m := testManager(t, func(w http.ResponseWriter, _ *http.Request) {
		close(entered)
		<-release
		_ = json.NewEncoder(w).Encode(fixtureRelease("v1.0.0", false, "2026-10-02T00:00:00Z"))
	})
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(release) }) })
	requestID := m.requestID
	done := make(chan error, 1)
	go func() {
		_, err := m.Start(context.Background(), "stable", "v1.0.0", requestID)
		done <- err
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("release check did not start")
	}
	resolved, err := m.Resolve(requestID)
	if err != nil || resolved.Job != nil || resolved.RequestID == requestID {
		t.Fatal(resolved, err)
	}
	once.Do(func() { close(release) })
	if err := <-done; !errors.Is(err, ErrRequest) {
		t.Fatal("delayed request queued after resolution", err)
	}
	if m.Busy() {
		t.Fatal("resolution queued a job")
	}
	job, err := m.Start(context.Background(), "stable", "v1.0.0", resolved.RequestID)
	if err != nil || job.ID != resolved.RequestID {
		t.Fatal("new confirmation was not accepted", job, err)
	}
}

func TestResolveFindsQueuedRunningAndFinishedJobs(t *testing.T) {
	for _, state := range []string{"pending", "running", "succeeded", "rolled_back", "recovery_required"} {
		t.Run(state, func(t *testing.T) {
			m := testManager(t, func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(fixtureRelease("v1.0.0", false, "2026-10-02T00:00:00Z"))
			})
			requestID := m.requestID
			job, err := m.Start(context.Background(), "stable", "v1.0.0", requestID)
			if err != nil {
				t.Fatal(err)
			}
			if state != "pending" {
				job.Status = state
				if err := atomicJSON(filepath.Join(m.dir, "status.json"), job, 0644); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(filepath.Join(m.dir, "requests", "request.json")); err != nil {
					t.Fatal(err)
				}
			}
			resolved, err := m.Resolve(requestID)
			if err != nil || resolved.Job == nil || resolved.Job.ID != requestID || resolved.Job.Status != state {
				t.Fatal(resolved, err)
			}
			if resolved.RequestID == requestID {
				t.Fatal("accepted credential was not consumed")
			}
			if _, err := m.Start(context.Background(), "stable", "v1.0.0", requestID); !errors.Is(err, ErrRequest) {
				t.Fatal("accepted request replayed", err)
			}
		})
	}
}

func TestResolveAndStartAreSerialized(t *testing.T) {
	m := testManager(t, func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(fixtureRelease("v1.0.0", false, "2026-10-02T00:00:00Z"))
	})
	if _, err := m.Check(context.Background(), "stable"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 40; i++ {
		status, err := m.Status()
		if err != nil {
			t.Fatal(err)
		}
		start := make(chan struct{})
		done := make(chan error, 1)
		go func() {
			<-start
			_, err := m.Start(context.Background(), "stable", "v1.0.0", status.RequestID)
			done <- err
		}()
		close(start)
		resolved, err := m.Resolve(status.RequestID)
		if err != nil {
			t.Fatal(err)
		}
		startErr := <-done
		if resolved.Job == nil {
			if !errors.Is(startErr, ErrRequest) || m.Busy() {
				t.Fatal("request queued after a negative resolution", startErr)
			}
		} else {
			if startErr != nil || resolved.Job.ID != status.RequestID {
				t.Fatal(resolved, startErr)
			}
			if err := os.Remove(filepath.Join(m.dir, "requests", "request.json")); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestCanceledOrRestartedRequestsCannotEnqueue(t *testing.T) {
	handler := func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(fixtureRelease("v1.0.0", false, "2026-10-02T00:00:00Z"))
	}
	m := testManager(t, handler)
	if _, err := m.Check(context.Background(), "stable"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := m.Start(ctx, "stable", "v1.0.0", m.requestID); !errors.Is(err, context.Canceled) || m.Busy() {
		t.Fatal("disconnected request queued", err)
	}
	restarted := testManager(t, handler)
	restarted.dir = m.dir
	if _, err := restarted.Start(context.Background(), "stable", "v1.0.0", m.requestID); !errors.Is(err, ErrRequest) {
		t.Fatal("old process credential accepted", err)
	}
	if _, err := m.Resolve("../../unsafe"); !errors.Is(err, ErrRequest) {
		t.Fatal(err)
	}
}

func TestQueueWriteFailureCanBeResolvedWithoutReplay(t *testing.T) {
	m := testManager(t, func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(fixtureRelease("v1.0.0", false, "2026-10-02T00:00:00Z"))
	})
	requestID := m.requestID
	inbox := filepath.Join(m.dir, "requests")
	if err := os.Remove(inbox); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Start(context.Background(), "stable", "v1.0.0", requestID); err == nil {
		t.Fatal("write unexpectedly succeeded")
	}
	resolved, err := m.Resolve(requestID)
	if err != nil || resolved.Job != nil || resolved.RequestID == requestID {
		t.Fatal(resolved, err)
	}
	if err := os.Mkdir(inbox, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Start(context.Background(), "stable", "v1.0.0", requestID); !errors.Is(err, ErrRequest) {
		t.Fatal("failed request replayed", err)
	}
	if _, err := m.Start(context.Background(), "stable", "v1.0.0", resolved.RequestID); err != nil {
		t.Fatal(err)
	}
}

func TestWorkerSuccessRollbackFailureAndReplay(t *testing.T) {
	for _, result := range []string{"succeeded", "rolled_back", "failed", "recovery_required"} {
		t.Run(result, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.Mkdir(filepath.Join(dir, "requests"), 0700); err != nil {
				t.Fatal(err)
			}
			request := filepath.Join(dir, "requests", "request.json")
			job := Job{ID: "0123456789abcdef0123456789abcdef", Version: "v1.0.0", Channel: "stable", CreatedAt: time.Now()}
			if err := atomicJSON(request, job, 0600); err != nil {
				t.Fatal(err)
			}
			check := func(context.Context, string) (Check, error) {
				return Check{Release: &Release{Version: "v1.0.0", PackageAvailable: true}}, nil
			}
			calls := 0
			run := func(version string, progress func(string) error) error {
				calls++
				if version != "v1.0.0" {
					t.Fatal(version)
				}
				if _, err := os.Stat(request); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("request not consumed")
				}
				var state Job
				if err := readJSON(filepath.Join(dir, "status.json"), &state, 8192); err != nil || state.Status != "running" {
					t.Fatal(state, err)
				}
				if err := progress("downloading"); err != nil {
					t.Fatal(err)
				}
				if result == "failed" {
					return errors.New("download failed")
				}
				if err := progress("installing"); err != nil {
					t.Fatal(err)
				}
				if result == "rolled_back" {
					_ = progress("restoring")
					_ = progress("restored")
					return errors.New("health check failed")
				}
				if result == "recovery_required" {
					return errors.New("restoration failed")
				}
				return nil
			}
			err := runWorker(dir, check, func() (string, error) { return "v1.0.0-rc.1", nil }, run)
			if (err == nil) != (result == "succeeded") {
				t.Fatal(result, err)
			}
			var saved Job
			if err := readJSON(filepath.Join(dir, "status.json"), &saved, 8192); err != nil || saved.Status != result {
				t.Fatal(saved, err)
			}
			if err := runWorker(dir, check, func() (string, error) { return "v1.0.0-rc.1", nil }, run); err == nil {
				t.Fatal("task replayed")
			}
			if calls != 1 {
				t.Fatal(calls)
			}
		})
	}
}

func TestWorkerRevalidatesVersionAndPreservesUnresolvedTask(t *testing.T) {
	dir := t.TempDir()
	_ = os.Mkdir(filepath.Join(dir, "requests"), 0700)
	request := filepath.Join(dir, "requests", "request.json")
	job := Job{ID: "0123456789abcdef0123456789abcdef", Version: "v1.0.0", Channel: "stable"}
	check := func(context.Context, string) (Check, error) { return Check{}, errors.New("offline") }
	run := func(string, func(string) error) error { t.Fatal("installer must not run"); return nil }
	for _, current := range []string{"v1.0.0", "v1.0.1", "dev", "v0.9.0"} {
		_ = atomicJSON(request, job, 0600)
		if err := runWorker(dir, check, func() (string, error) { return current, nil }, run); err == nil {
			t.Fatal(current)
		}
	}
	job.Status = "running"
	_ = atomicJSON(filepath.Join(dir, "status.json"), job, 0644)
	_ = atomicJSON(request, job, 0600)
	if err := runWorker(dir, check, func() (string, error) { return "v0.9.0", nil }, run); !errors.Is(err, ErrBusy) {
		t.Fatal(err)
	}
}

func TestReadJSONRejectsNonRegularAndOversizedFiles(t *testing.T) {
	dir := t.TempDir()
	var value Job
	if err := readJSON(dir, &value, 4096); err == nil {
		t.Fatal("directory accepted")
	}
	path := filepath.Join(dir, "request.json")
	if err := os.WriteFile(path, make([]byte, 4097), 0600); err != nil {
		t.Fatal(err)
	}
	if err := readJSON(path, &value, 4096); err == nil {
		t.Fatal("oversized file accepted")
	}
	if err := os.Symlink(path, filepath.Join(dir, "link")); err == nil {
		if err := readJSON(filepath.Join(dir, "link"), &value, 4096); err == nil {
			t.Fatal("symlink accepted")
		}
	}
}
