package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/gorilla/websocket"
	"github.com/liyiligang/loom/internal/controller"
	"github.com/liyiligang/loom/internal/credentials"
	"github.com/liyiligang/loom/internal/discovery"
	"github.com/liyiligang/loom/internal/httpapi"
	"github.com/liyiligang/loom/internal/local"
	"github.com/liyiligang/loom/internal/model"
	"github.com/liyiligang/loom/internal/storage"
)

// The fixture speaks the native protocol over a real WebSocket. Tests enter Loom
// through HTTP and inspect its published snapshots and event replay only.
type nativeTurn struct {
	ID     string           `json:"id"`
	Status string           `json:"status"`
	Items  []map[string]any `json:"items"`
}

type nativeThread struct {
	Model string       `json:"-"`
	ID    string       `json:"id"`
	Cwd   string       `json:"cwd"`
	Turns []nativeTurn `json:"turns"`
}

type peer struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func (p *peer) write(value any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	_ = p.conn.WriteJSON(value)
}

type simulatedCodex struct {
	server        *httptest.Server
	mu            sync.Mutex
	current       *peer
	peers         []*peer
	threads       map[string]*nativeThread
	starts        int
	loseAck       string
	createStarted chan struct{}
	createRelease chan struct{}
	replies       chan map[string]any
	requests      []nativeRequest
}

type nativeRequest struct {
	Method string
	Params json.RawMessage
}

func newSimulatedCodex(t *testing.T) *simulatedCodex {
	f := &simulatedCodex{threads: make(map[string]*nativeThread), replies: make(chan map[string]any, 16)}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-service-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		p := &peer{conn: conn}
		f.mu.Lock()
		f.current = p
		f.peers = append(f.peers, p)
		f.mu.Unlock()
		defer conn.Close()
		for {
			var packet struct {
				ID     json.RawMessage `json:"id"`
				Method string          `json:"method"`
				Params json.RawMessage `json:"params"`
				Result map[string]any  `json:"result"`
			}
			if conn.ReadJSON(&packet) != nil {
				return
			}
			if packet.Method == "" {
				if packet.Result != nil {
					f.replies <- packet.Result
				}
				continue
			}
			var params struct {
				Cwd          string `json:"cwd"`
				Model        string `json:"model"`
				ThreadID     string `json:"threadId"`
				IncludeTurns bool   `json:"includeTurns"`
				Input        []struct {
					Text string `json:"text"`
				} `json:"input"`
			}
			_ = json.Unmarshal(packet.Params, &params)
			if packet.Method == "thread/start" {
				f.mu.Lock()
				started, release := f.createStarted, f.createRelease
				f.mu.Unlock()
				if started != nil {
					started <- struct{}{}
					<-release
				}
			}
			f.mu.Lock()
			f.requests = append(f.requests, nativeRequest{Method: packet.Method, Params: append(json.RawMessage(nil), packet.Params...)})
			var result any
			closeConnection := false
			switch packet.Method {
			case "initialize":
				result = map[string]any{"userAgent": "codex-test/0.125.0"}
			case "initialized":
				f.mu.Unlock()
				continue
			case "thread/start":
				thread := &nativeThread{ID: fmt.Sprintf("thread-%d", len(f.threads)+1), Cwd: params.Cwd, Turns: []nativeTurn{}}
				thread.Model = params.Model
				if thread.Model == "" {
					thread.Model = "test-model"
				}
				f.threads[thread.ID] = thread
				result = map[string]any{"thread": thread, "model": thread.Model}
			case "thread/read", "thread/resume":
				thread := f.threads[params.ThreadID]
				if packet.Method == "thread/resume" && len(thread.Turns) == 0 {
					p.write(map[string]any{"id": packet.ID, "error": map[string]any{"code": -32000, "message": "no rollout found"}})
					f.mu.Unlock()
					continue
				}
				copy := *thread
				if packet.Method == "thread/read" && !params.IncludeTurns {
					copy.Turns = nil
				}
				result = map[string]any{"thread": copy, "model": thread.Model}
			case "model/list":
				result = map[string]any{"data": []map[string]any{{"model": "test-model", "displayName": "Fixture default", "isDefault": true}, {"model": "project-model", "displayName": "Project model", "isDefault": false}}}
			case "config/read":
				result = map[string]any{"config": map[string]any{"model": "test-model"}}
			case "turn/start":
				f.starts++
				thread := f.threads[params.ThreadID]
				if params.Model != "" {
					thread.Model = params.Model
				}
				turn := nativeTurn{ID: fmt.Sprintf("turn-%d", f.starts), Status: "inProgress", Items: []map[string]any{
					{"id": fmt.Sprintf("user-%d", f.starts), "type": "userMessage", "content": []map[string]any{{"type": "text", "text": params.Input[0].Text}}},
				}}
				if f.loseAck != "" {
					turn.Status = "completed"
					turn.Items = append(turn.Items, map[string]any{"id": "recovered", "type": "agentMessage", "text": "Recovered answer"})
					closeConnection = true
				}
				thread.Turns = append(thread.Turns, turn)
				if f.loseAck == "ambiguous" {
					thread.Turns = append(thread.Turns, nativeTurn{ID: "other-writer", Status: "completed", Items: []map[string]any{}})
				}
				f.loseAck = ""
				result = map[string]any{"turn": turn}
			case "turn/interrupt":
				result = map[string]any{}
			default:
				result = map[string]any{}
			}
			if !closeConnection {
				p.write(map[string]any{"id": packet.ID, "result": result})
			}
			f.mu.Unlock()
			if closeConnection {
				return
			}
		}
	}))
	t.Cleanup(func() {
		f.mu.Lock()
		for _, p := range f.peers {
			p.conn.Close()
		}
		f.mu.Unlock()
		f.server.Close()
	})
	return f
}

func (f *simulatedCodex) send(method string, params map[string]any, requestID string) {
	f.mu.Lock()
	p := f.current
	f.mu.Unlock()
	packet := map[string]any{"method": method, "params": params}
	if requestID != "" {
		packet["id"] = requestID
	}
	p.write(packet)
}

func (f *simulatedCodex) finish(threadID, turnID, state, text string) {
	f.mu.Lock()
	var completed nativeTurn
	for i := range f.threads[threadID].Turns {
		turn := &f.threads[threadID].Turns[i]
		if turn.ID == turnID {
			turn.Status = state
			turn.Items = append(turn.Items, map[string]any{"id": "answer-" + turnID, "type": "agentMessage", "text": text})
			completed = *turn
		}
	}
	f.mu.Unlock()
	f.send("turn/completed", map[string]any{"threadId": threadID, "turn": completed}, "")
}

type detail struct {
	model.Detail
	Cursor  int64 `json:"cursor"`
	Partial bool  `json:"partial"`
}

type fixture struct {
	t        *testing.T
	native   *simulatedCodex
	server   *httptest.Server
	service  *controller.Service
	scanner  *discovery.Scanner
	store    *storage.Store
	dataPath string
	manager  local.Manager
	endpoint string
}

func setup(t *testing.T) *fixture {
	f := &fixture{t: t, native: newSimulatedCodex(t), dataPath: t.TempDir()}
	f.start()
	t.Cleanup(f.stop)
	var saved struct{ ID string }
	f.call("POST", "/endpoints", map[string]any{"name": "Test agent", "address": "ws" + strings.TrimPrefix(f.native.server.URL, "http"), "token": "test-service-token"}, 202, &saved)
	f.endpoint = saved.ID
	f.waitReady()
	return f
}

func (f *fixture) start() {
	f.t.Helper()
	var err error
	f.store, err = storage.Open(filepath.Join(f.dataPath, "loom.db"))
	if err != nil {
		f.t.Fatal(err)
	}
	secrets, err := credentials.New(filepath.Join(f.dataPath, "credentials"))
	if err != nil {
		f.t.Fatal(err)
	}
	var options []controller.Option
	if f.manager != nil {
		options = append(options, controller.WithLocalManager(f.manager))
	}
	f.service, err = controller.New(context.Background(), f.store, secrets, options...)
	if err != nil {
		f.t.Fatal(err)
	}
	f.scanner = discovery.New(context.Background(), []int{3000})
	f.server = httptest.NewUnstartedServer(nil)
	f.server.Config.Handler = httpapi.New(context.Background(), f.service, f.scanner, fstest.MapFS{"index.html": {Data: []byte("test")}}, f.server.Listener.Addr().String())
	f.server.Start()
	f.service.Start()
}

func (f *fixture) stop() {
	f.server.Close()
	f.service.Close()
	f.scanner.Close()
	f.store.Close()
}

func (f *fixture) call(method, path string, input any, want int, result any) {
	f.t.Helper()
	status, raw, err := f.request(method, path, input)
	if err != nil {
		f.t.Fatal(err)
	}
	if status != want {
		f.t.Fatalf("%s %s: status %d, want %d: %s", method, path, status, want, raw)
	}
	if result != nil && json.Unmarshal(raw, result) != nil {
		f.t.Fatalf("invalid response: %s", raw)
	}
}

func (f *fixture) request(method, path string, input any) (int, []byte, error) {
	body, _ := json.Marshal(input)
	request, _ := http.NewRequest(method, f.server.URL+"/api/v1"+path, bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response, err := f.server.Client().Do(request)
	if err != nil {
		return 0, nil, err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	return response.StatusCode, raw, err
}

func eventually(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("published state did not reach the expected condition")
}

func (f *fixture) waitReady() {
	eventually(f.t, func() bool {
		var bootstrap struct{ Endpoints []model.Endpoint }
		f.call("GET", "/bootstrap", nil, 200, &bootstrap)
		return len(bootstrap.Endpoints) == 1 && bootstrap.Endpoints[0].State == "ready"
	})
}

func (f *fixture) session() string {
	var result struct{ ID string }
	f.call("POST", "/sessions", map[string]any{"endpointId": f.endpoint, "cwd": "/target/project"}, 201, &result)
	return result.ID
}

func (f *fixture) snapshot(id string) detail {
	var result detail
	f.call("GET", "/sessions/"+id, nil, 200, &result)
	return result
}

func (f *fixture) submit(id, requestID, text string) detail {
	f.call("POST", "/sessions/"+id+"/turns", map[string]any{"clientRequestId": requestID, "text": text}, 202, nil)
	var result detail
	eventually(f.t, func() bool {
		result = f.snapshot(id)
		for _, turn := range result.Turns {
			if turn.ClientRequestID == requestID && turn.ProviderID != "" {
				return true
			}
		}
		return false
	})
	return result
}

func TestConcurrentSubmissionIsDeduplicated(t *testing.T) {
	f := setup(t)
	id := f.session()
	var workers sync.WaitGroup
	statuses := make(chan int, 8)
	for range 8 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			status, _, _ := f.request("POST", "/sessions/"+id+"/turns", map[string]any{"clientRequestId": "same-request", "text": "Build the feature"})
			statuses <- status
		}()
	}
	workers.Wait()
	close(statuses)
	for status := range statuses {
		if status != 202 {
			t.Fatalf("duplicate submission returned %d", status)
		}
	}
	eventually(t, func() bool { f.native.mu.Lock(); defer f.native.mu.Unlock(); return f.native.starts == 1 })
	f.call("POST", "/sessions/"+id+"/turns", map[string]any{"clientRequestId": "same-request", "text": "Different text"}, 409, nil)
	f.call("POST", "/sessions/"+id+"/turns", map[string]any{"clientRequestId": "new-request", "text": "Second task"}, 409, nil)
	if len(f.snapshot(id).Turns) != 1 {
		t.Fatal("duplicate submissions created multiple turns")
	}
}

func TestConcurrentSessionsKeepOutputSeparateAndBatchUpdates(t *testing.T) {
	f := setup(t)
	a, b := f.session(), f.session()
	first := f.submit(a, "first", "Task A")
	second := f.submit(b, "second", "Task B")
	for range 100 {
		f.native.send("item/agentMessage/delta", map[string]any{"threadId": first.Session.ProviderID, "turnId": first.Turns[0].ProviderID, "itemId": "answer-a", "delta": "a"}, "")
		f.native.send("item/agentMessage/delta", map[string]any{"threadId": second.Session.ProviderID, "turnId": second.Turns[0].ProviderID, "itemId": "answer-b", "delta": "b"}, "")
	}
	var current detail
	eventually(t, func() bool {
		current = f.snapshot(a)
		return len(current.Entries) == 2 && current.Entries[1].Text == strings.Repeat("a", 100)
	})
	if current.Cursor-first.Cursor >= 50 {
		t.Fatalf("output was not batched: cursor advanced by %d", current.Cursor-first.Cursor)
	}
	eventually(t, func() bool {
		other := f.snapshot(b)
		return len(other.Entries) == 2 && other.Entries[1].Text == strings.Repeat("b", 100)
	})
	var changes detail
	f.call("GET", fmt.Sprintf("/sessions/%s?afterCursor=%d", a, first.Cursor), nil, 200, &changes)
	if !changes.Partial || len(changes.Entries) != 1 || changes.Entries[0].Text != strings.Repeat("a", 100) {
		t.Fatalf("incremental snapshot repeated unchanged history: %+v", changes)
	}
	address := "ws" + strings.TrimPrefix(f.server.URL, "http") + fmt.Sprintf("/api/v1/events?afterCursor=%d", first.Cursor)
	conn, _, err := websocket.DefaultDialer.Dial(address, http.Header{"Origin": []string{f.server.URL}})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	var event model.Event
	if conn.ReadJSON(&event) != nil || event.ID <= first.Cursor {
		t.Fatal("WebSocket replay did not continue after the snapshot cursor")
	}
}

func TestApprovalAndInterruptionWaitForNativeConfirmation(t *testing.T) {
	f := setup(t)
	id := f.session()
	initial := f.submit(id, "approval-task", "Run a command")
	thread, turn := initial.Session.ProviderID, initial.Turns[0].ProviderID
	f.native.send("item/commandExecution/requestApproval", map[string]any{"threadId": thread, "turnId": turn, "itemId": "command", "command": "git status"}, "approval-1")
	var interactionID string
	eventually(t, func() bool {
		current := f.snapshot(id)
		if len(current.Interactions) == 1 {
			interactionID = current.Interactions[0].ID
		}
		return interactionID != "" && current.Session.State == "waiting_approval"
	})
	f.call("POST", "/interactions/"+interactionID+"/respond", map[string]any{"decision": "accept"}, 200, nil)
	select {
	case reply := <-f.native.replies:
		if reply["decision"] != "accept" {
			t.Fatalf("incorrect approval reply: %v", reply)
		}
	case <-time.After(time.Second):
		t.Fatal("native service received no approval reply")
	}
	if f.snapshot(id).Interactions[0].State != "replying" {
		t.Fatal("approval was resolved before native confirmation")
	}
	f.call("POST", "/interactions/"+interactionID+"/respond", map[string]any{"decision": "accept"}, 409, nil)
	f.native.send("serverRequest/resolved", map[string]any{"threadId": thread, "requestId": "approval-1"}, "")
	eventually(t, func() bool { return f.snapshot(id).Interactions[0].State == "resolved" })
	f.call("POST", "/turns/"+initial.Turns[0].ID+"/interrupt", nil, 200, nil)
	if f.snapshot(id).Session.State != "running" {
		t.Fatal("interruption was marked complete before native confirmation")
	}
	f.native.finish(thread, turn, "interrupted", "Stopped")
	eventually(t, func() bool { return f.snapshot(id).Session.State == "idle" })
}

func TestQuestionsRequireAnswersAndTerminalEventsCloseRequests(t *testing.T) {
	f := setup(t)
	id := f.session()
	initial := f.submit(id, "question-task", "Choose an option")
	f.native.send("item/tool/requestUserInput", map[string]any{"threadId": initial.Session.ProviderID, "turnId": initial.Turns[0].ProviderID,
		"questions": []map[string]any{{"id": "choice", "question": "Which option?"}}}, "question-1")
	eventually(t, func() bool { return f.snapshot(id).Session.State == "waiting_input" })
	interaction := f.snapshot(id).Interactions[0]
	f.call("POST", "/interactions/"+interaction.ID+"/respond", map[string]any{"answers": map[string]any{}}, 400, nil)
	f.call("POST", "/interactions/"+interaction.ID+"/respond", map[string]any{"answers": map[string]any{"choice": map[string]any{"answers": []string{"Option A"}}}}, 200, nil)
	f.native.finish(initial.Session.ProviderID, initial.Turns[0].ProviderID, "completed", "Done")
	eventually(t, func() bool { return f.snapshot(id).Interactions[0].State == "cancelled" })
	f.call("POST", "/interactions/"+interaction.ID+"/respond", map[string]any{"answers": map[string]any{}}, 409, nil)
}

func TestLostAcknowledgementRecoversWithoutResubmission(t *testing.T) {
	f := setup(t)
	id := f.session()
	f.native.mu.Lock()
	f.native.loseAck = "identifiable"
	f.native.mu.Unlock()
	f.call("POST", "/sessions/"+id+"/turns", map[string]any{"clientRequestId": "lost-ack", "text": "Recover this task"}, 202, nil)
	eventually(t, func() bool {
		current := f.snapshot(id)
		return current.Session.State == "idle" && len(current.Turns) == 1 && current.Turns[0].State == "completed" && len(current.Entries) == 2 && current.Entries[1].Text == "Recovered answer"
	})
	f.call("POST", "/sessions/"+id+"/turns", map[string]any{"clientRequestId": "lost-ack", "text": "Recover this task"}, 202, nil)
	f.native.mu.Lock()
	defer f.native.mu.Unlock()
	if f.native.starts != 1 {
		t.Fatalf("task was submitted %d times", f.native.starts)
	}
}

func TestConnectionLossFlushesReceivedOutputBeforeRecovery(t *testing.T) {
	f := setup(t)
	id := f.session()
	initial := f.submit(id, "stream-disconnect", "Retain streamed output")
	for range 50 {
		f.native.send("item/agentMessage/delta", map[string]any{"threadId": initial.Session.ProviderID,
			"turnId": initial.Turns[0].ProviderID, "itemId": "transient-answer", "delta": "x"}, "")
	}
	f.native.mu.Lock()
	f.native.current.conn.Close()
	f.native.mu.Unlock()
	eventually(t, func() bool {
		current := f.snapshot(id)
		return len(current.Entries) == 2 && current.Entries[1].Text == strings.Repeat("x", 50)
	})
}

func TestExpiredApprovalCannotBeAnsweredAfterReconnect(t *testing.T) {
	f := setup(t)
	id := f.session()
	initial := f.submit(id, "stale-approval", "Check connection generations")
	params := map[string]any{"threadId": initial.Session.ProviderID, "turnId": initial.Turns[0].ProviderID, "command": "git status"}
	f.native.send("item/commandExecution/requestApproval", params, "reused-request-id")
	eventually(t, func() bool { return f.snapshot(id).Session.State == "waiting_approval" })
	oldID := f.snapshot(id).Interactions[0].ID
	f.native.mu.Lock()
	f.native.current.conn.Close()
	f.native.mu.Unlock()
	eventually(t, func() bool { return f.snapshot(id).Interactions[0].State == "unknown" })
	f.waitReady()
	f.call("POST", "/interactions/"+oldID+"/respond", map[string]any{"decision": "accept"}, 409, nil)
	f.native.send("item/commandExecution/requestApproval", params, "reused-request-id")
	eventually(t, func() bool { return len(f.snapshot(id).Interactions) == 2 })
	current := f.snapshot(id)
	if current.Interactions[0].State != "unknown" || current.Interactions[1].State != "pending" {
		t.Fatal("the new connection reused an expired approval")
	}
}

func TestTerminalErrorClosesPendingInteractions(t *testing.T) {
	f := setup(t)
	id := f.session()
	initial := f.submit(id, "failed-approval", "Report a terminal failure")
	params := map[string]any{"threadId": initial.Session.ProviderID, "turnId": initial.Turns[0].ProviderID}
	f.native.send("item/commandExecution/requestApproval", params, "failed-request")
	eventually(t, func() bool { return len(f.snapshot(id).Interactions) == 1 })
	f.native.send("error", map[string]any{"threadId": initial.Session.ProviderID, "turnId": initial.Turns[0].ProviderID,
		"error": map[string]any{"message": "Execution failed"}, "willRetry": false}, "")
	eventually(t, func() bool {
		current := f.snapshot(id)
		return current.Session.State == "idle" && current.Turns[0].State == "failed" && current.Interactions[0].State == "cancelled"
	})
}

func TestEmptySessionRecoveryExplainsTheNativeRolloutLimitation(t *testing.T) {
	f := setup(t)
	id := f.session()
	f.native.mu.Lock()
	f.native.current.conn.Close()
	f.native.mu.Unlock()
	f.waitReady()
	eventually(t, func() bool {
		current := f.snapshot(id)
		return current.Session.State == "syncing" && strings.Contains(current.Session.Error, "cannot restore an empty session")
	})
	f.call("POST", "/sessions/"+id+"/restore", nil, 502, nil)
	f.call("POST", "/sessions/"+id+"/turns", map[string]any{"clientRequestId": "not-resumed", "text": "New task"}, 409, nil)
}

func TestLateDeltasAndStaleSnapshotsCannotOverwriteFinalOutput(t *testing.T) {
	f := setup(t)
	id := f.session()
	initial := f.submit(id, "finality", "Preserve the final answer")
	thread, turn := initial.Session.ProviderID, initial.Turns[0].ProviderID
	f.native.finish(thread, turn, "completed", "Final answer")
	eventually(t, func() bool { return f.snapshot(id).Session.State == "idle" })
	completedCursor := f.snapshot(id).Cursor
	f.native.send("item/agentMessage/delta", map[string]any{"threadId": thread, "turnId": turn, "itemId": "answer-" + turn, "delta": "Late output"}, "")
	f.native.send("turn/started", map[string]any{"threadId": thread, "turn": map[string]any{"id": turn}}, "")
	eventually(t, func() bool { return f.snapshot(id).Cursor >= completedCursor+2 })
	f.native.mu.Lock()
	native := &f.native.threads[thread].Turns[0]
	native.Status = "inProgress"
	native.Items[len(native.Items)-1]["text"] = "Final"
	f.native.mu.Unlock()
	f.call("POST", "/sessions/"+id+"/restore", nil, 200, nil)
	current := f.snapshot(id)
	if current.Session.State != "idle" || current.Entries[1].Text != "Final answer" || current.Entries[1].Status != "completed" {
		t.Fatalf("stale native state rolled back the final output: %+v", current)
	}
}

func TestAmbiguousRecoveryBlocksFurtherExecution(t *testing.T) {
	f := setup(t)
	id := f.session()
	f.native.mu.Lock()
	f.native.loseAck = "ambiguous"
	f.native.mu.Unlock()
	f.call("POST", "/sessions/"+id+"/turns", map[string]any{"clientRequestId": "ambiguous", "text": "Do not duplicate"}, 202, nil)
	eventually(t, func() bool {
		current := f.snapshot(id)
		return len(current.Turns) == 3 && current.Session.State == "syncing"
	})
	f.call("POST", "/sessions/"+id+"/turns", map[string]any{"clientRequestId": "another", "text": "Next task"}, 409, nil)
}

func TestRestartRetainsHistoryAndResynchronizesOldCursors(t *testing.T) {
	f := setup(t)
	id := f.session()
	initial := f.submit(id, "persisted", "Keep this history")
	f.native.finish(initial.Session.ProviderID, initial.Turns[0].ProviderID, "completed", "Persistent answer")
	eventually(t, func() bool { return f.snapshot(id).Session.State == "idle" })
	before := f.snapshot(id)
	f.stop()
	f.start()
	f.waitReady()
	eventually(t, func() bool { return f.snapshot(id).Session.State == "idle" })
	var recovered detail
	f.call("GET", fmt.Sprintf("/sessions/%s?afterCursor=%d", id, before.Cursor), nil, 200, &recovered)
	if len(recovered.Entries) != 2 || recovered.Entries[1].Text != "Persistent answer" {
		t.Fatalf("restart lost history or did not refresh old cursors: %+v", recovered)
	}
}

func TestHTTPRejectsUntrustedOriginsAndInvalidCursors(t *testing.T) {
	f := setup(t)
	id := f.session()
	request, _ := http.NewRequest("GET", f.server.URL+"/api/v1/bootstrap", nil)
	request.Header.Set("Origin", "https://untrusted.example")
	response, err := f.server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 403 {
		t.Fatalf("untrusted origin returned %d", response.StatusCode)
	}
	f.call("GET", "/sessions/"+id+"?afterCursor=-1", nil, 400, nil)
	f.call("GET", "/sessions/"+id+"?afterCursor=invalid", nil, 400, nil)
	f.call("GET", "/sessions/"+id+"?afterCursor=999999", nil, 409, nil)
}

func TestDefaultTitleMetadataPreservesExplicitSessionNames(t *testing.T) {
	f := setup(t)
	id := f.session()
	created := f.snapshot(id)
	if created.Session.DefaultTitle == nil || !*created.Session.DefaultTitle || created.Session.Title != "New session" {
		t.Fatal("generated session title was not identified for localization")
	}
	f.call("PATCH", "/sessions/"+id, map[string]any{"title": "New session"}, 200, nil)
	renamed := f.snapshot(id)
	if renamed.Session.DefaultTitle == nil || *renamed.Session.DefaultTitle || renamed.Session.Title != "New session" {
		t.Fatal("an explicitly chosen name was incorrectly treated as a generated title")
	}
}
