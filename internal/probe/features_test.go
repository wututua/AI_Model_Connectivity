package probe

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"cg/internal/config"
)

func TestDiscoveryThresholdRequiresExplicitModels(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			fmt.Fprint(w, `{"data":[{"id":"a"},{"id":"b"}]}`)
			return
		}
		calls.Add(1)
		fmt.Fprint(w, `{"choices":[{"message":{"content":"pang"}}]}`)
	}))
	defer server.Close()
	cfg := config.Config{TimeoutSeconds: 2, ModelListTimeoutSeconds: 2, OperationsSettings: config.OperationsSettings{DiscoveryModelLimit: 1},
		Providers: []config.ProviderConfig{{ID: "p", BaseURL: server.URL, Enabled: true, ProbeEnabled: true}}}
	results, failures, err := NewRunner(cfg).Run(context.Background())
	if err != nil || len(results) != 0 || len(failures) != 1 || calls.Load() != 0 {
		t.Fatal(results, failures, err)
	}
	cfg.Providers[0].Models = []string{"a", "b"}
	var snapshots []Progress
	runner := NewRunner(cfg)
	runner.SetObserver(func(p Progress) { snapshots = append(snapshots, p) })
	results, failures, err = runner.Run(context.Background())
	if err != nil || len(results) != 2 || len(failures) != 0 || calls.Load() != 2 {
		t.Fatal(results, failures, err)
	}
	last := snapshots[len(snapshots)-1]
	if last.Total != 2 || last.Completed != 2 || len(last.Active) != 0 {
		t.Fatal(last)
	}
}
