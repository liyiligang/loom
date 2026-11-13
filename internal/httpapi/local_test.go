package httpapi_test

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/liyiligang/loom/internal/controller"
	"github.com/liyiligang/loom/internal/local"
	"github.com/liyiligang/loom/internal/model"
	"github.com/liyiligang/loom/internal/provider"
	"github.com/liyiligang/loom/internal/provider/codex"
)

type localManager struct {
	agents  []local.Agent
	gate    chan struct{}
	address string
	scans   atomic.Int32
	starts  atomic.Int32
}

func (m *localManager) Scan(ctx context.Context) []local.Agent {
	m.scans.Add(1)
	if m.gate != nil {
		select {
		case <-m.gate:
		case <-ctx.Done():
			return nil
		}
	}
	return append([]local.Agent{}, m.agents...)
}
func (m *localManager) Connect(ctx context.Context, agent local.Agent) (provider.Connection, error) {
	m.starts.Add(1)
	return codex.Dial(ctx, m.address, "test-service-token")
}
func localFixture(t *testing.T, available bool, gate chan struct{}) (*fixture, *localManager) {
	f := &fixture{t: t, native: newSimulatedCodex(t), dataPath: t.TempDir()}
	manager := &localManager{gate: gate, address: "ws" + strings.TrimPrefix(f.native.server.URL, "http"), agents: []local.Agent{
		{Provider: "codex", Name: "Codex", State: "missing"}, {Provider: "claude", Name: "Claude Code", State: "missing"},
	}}
	if available {
		manager.agents[0].State = "available"
		manager.agents[0].Path = "/detected/codex"
		manager.agents[0].Version = "codex-cli 0.125.0"
	}
	f.manager = manager
	f.start()
	t.Cleanup(f.stop)
	return f, manager
}

type localBootstrap struct {
	Local     controller.LocalSnapshot
	Endpoints []model.Endpoint
}

func (f *fixture) localReady() localBootstrap {
	var result localBootstrap
	eventually(f.t, func() bool { f.call("GET", "/bootstrap", nil, 200, &result); return result.Local.State == "ready" })
	return result
}
func TestLocalAgentsRequireOnePersistedConfirmation(t *testing.T) {
	f, manager := localFixture(t, true, nil)
	initial := f.localReady()
	if initial.Local.Confirmed || len(initial.Endpoints) != 0 || manager.starts.Load() != 0 {
		t.Fatal("detection started a connection before confirmation")
	}
	f.call("GET", "/local/directories", nil, 409, nil)
	var wg sync.WaitGroup
	statuses := make(chan int, 12)
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			status, _, err := f.request("POST", "/local/confirm", nil)
			if err != nil {
				status = 0
			}
			statuses <- status
		}()
	}
	wg.Wait()
	close(statuses)
	for status := range statuses {
		if status != 200 {
			t.Fatalf("concurrent confirmation: %d", status)
		}
	}
	f.endpoint = "local-codex"
	f.waitReady()
	if manager.starts.Load() != 1 {
		t.Fatalf("confirmation started %d connections", manager.starts.Load())
	}
	f.call("PATCH", "/endpoints/local-codex", map[string]any{"name": "Changed", "address": "ws://127.0.0.1:1234/", "token": "new"}, 409, nil)
	ready := f.localReady()
	if len(ready.Endpoints) != 1 || !ready.Endpoints[0].Local || ready.Endpoints[0].HasToken {
		t.Fatal("managed endpoint accepted manual configuration")
	}
	f.stop()
	f.start()
	f.waitReady()
	restored := f.localReady()
	if !restored.Local.Confirmed || manager.starts.Load() != 2 {
		t.Fatal("restart did not restore confirmation and reconnect")
	}
	f.call("POST", "/endpoints/local-codex/disconnect", nil, 200, nil)
	f.call("POST", "/local/rescan", nil, 202, nil)
	_ = f.localReady()
	if manager.starts.Load() != 2 {
		t.Fatal("rescan re-enabled a disconnected agent")
	}
}
func TestLocalScanningMissingAgentsAndOriginGuard(t *testing.T) {
	gate := make(chan struct{})
	f, manager := localFixture(t, false, gate)
	f.call("POST", "/local/confirm", nil, 409, nil)
	f.call("POST", "/local/rescan", nil, 202, nil)
	close(gate)
	_ = f.localReady()
	if manager.scans.Load() != 1 {
		t.Fatal("overlapping scans were not deduplicated")
	}
	f.call("POST", "/local/confirm", nil, 409, nil)
	if manager.starts.Load() != 0 {
		t.Fatal("missing installation started an agent")
	}
	request, _ := http.NewRequest("POST", f.server.URL+"/api/v1/local/confirm", strings.NewReader("{}"))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "https://untrusted.example")
	response, err := f.server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 403 {
		t.Fatal("untrusted website could confirm local agents")
	}
}
func TestLocalProjectDirectorySelectionAndRemoteException(t *testing.T) {
	f, _ := localFixture(t, true, nil)
	_ = f.localReady()
	f.call("POST", "/local/confirm", nil, 200, nil)
	f.waitReady()
	directory := t.TempDir()
	if err := os.Mkdir(filepath.Join(directory, "project"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "private.txt"), []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	var listing controller.DirectoryListing
	f.call("GET", "/local/directories?path="+directory, nil, 200, &listing)
	if len(listing.Directories) != 1 || listing.Directories[0].Name != "project" || listing.Path != directory {
		t.Fatalf("unexpected directory listing: %+v", listing)
	}
	var saved struct{ ID string }
	f.call("POST", "/sessions", map[string]any{"endpointId": "local-codex", "cwd": listing.Directories[0].Path}, 201, &saved)
	var session detail
	f.call("GET", "/sessions/"+saved.ID, nil, 200, &session)
	if session.Session.Cwd != filepath.Join(directory, "project") || session.Session.Title != "New session" {
		t.Fatal("local session required additional fields")
	}
	f.call("POST", "/endpoints", map[string]any{"name": "Remote agent", "address": "ws" + strings.TrimPrefix(f.native.server.URL, "http") + "/remote", "token": "test-service-token"}, 202, &saved)
	bootstrap := f.localReady()
	if len(bootstrap.Endpoints) != 2 {
		t.Fatal("manual remote connection was not retained")
	}
}
