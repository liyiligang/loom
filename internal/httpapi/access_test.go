package httpapi_test

import (
	"encoding/json"
	"testing"

	"github.com/liyiligang/loom/internal/model"
	"github.com/liyiligang/loom/internal/provider"
)

func assertNativeAccess(t *testing.T, f *fixture, method, approval, sandbox string) {
	t.Helper()
	f.native.mu.Lock()
	defer f.native.mu.Unlock()
	for i := len(f.native.requests) - 1; i >= 0; i-- {
		request := f.native.requests[i]
		if request.Method != method {
			continue
		}
		var params struct {
			ApprovalPolicy string `json:"approvalPolicy"`
			Sandbox        string `json:"sandbox"`
			SandboxPolicy  struct {
				Type string `json:"type"`
			} `json:"sandboxPolicy"`
		}
		if err := json.Unmarshal(request.Params, &params); err != nil {
			t.Fatal(err)
		}
		actual := params.Sandbox
		if method == "turn/start" {
			actual = params.SandboxPolicy.Type
		}
		if params.ApprovalPolicy != approval || actual != sandbox {
			t.Fatalf("%s used %s/%s, expected %s/%s", method, params.ApprovalPolicy, actual, approval, sandbox)
		}
		return
	}
	t.Fatalf("missing %s request", method)
}

func TestAccessRestoreRetainsActiveTurnPolicyAfterPreferenceChanges(t *testing.T) {
	f := setup(t)
	id := f.session()
	f.call("PATCH", "/sessions/"+id, map[string]any{"accessMode": "full"}, 200, nil)
	before := f.submit(id, "access-running", "Run the fixture task")
	f.call("PATCH", "/sessions/"+id, map[string]any{"accessMode": "workspace"}, 200, nil)
	f.stop()
	f.start()
	f.waitReady()
	eventually(t, func() bool { return f.snapshot(id).Session.State == "running" })
	assertNativeAccess(t, f, "thread/resume", "never", "danger-full-access")
	current := f.snapshot(id)
	if current.Session.AccessMode != provider.AccessWorkspace || current.Turns[0].AccessMode != provider.AccessFull || current.Session.ActiveTurn != before.Session.ActiveTurn {
		t.Fatal("restoration replaced the active policy with the next-task preference")
	}
	f.native.finish(current.Session.ProviderID, current.Turns[0].ProviderID, "completed", "Fixture task complete")
	eventually(t, func() bool { return f.snapshot(id).Session.State == "idle" })
	f.submit(id, "access-after-restore", "Run with workspace access")
	assertNativeAccess(t, f, "turn/start", "on-request", "workspaceWrite")
}

func TestAccessChangeDuringLazyConversationCreationReachesFirstTurn(t *testing.T) {
	f := setup(t)
	id := requirementForChat(t, f)
	started, release := make(chan struct{}, 1), make(chan struct{})
	f.native.mu.Lock()
	f.native.createStarted, f.native.createRelease = started, release
	f.native.mu.Unlock()
	done := make(chan struct {
		status int
		body   []byte
		err    error
	}, 1)
	go func() {
		status, body, err := f.request("POST", "/requirements/"+id+"/conversation/turns", map[string]any{"text": "Align design", "clientRequestId": "access-during-creation", "section": "designDocuments"})
		done <- struct {
			status int
			body   []byte
			err    error
		}{status, body, err}
	}()
	<-started
	f.call("PATCH", "/requirements/"+id+"/conversation/access", map[string]any{"accessMode": "full"}, 200, nil)
	close(release)
	result := <-done
	if result.err != nil || result.status != 202 {
		t.Fatalf("conversation submission failed: %d %v", result.status, result.err)
	}
	var sent struct{ SessionID, TurnID string }
	if err := json.Unmarshal(result.body, &sent); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool { return f.snapshot(sent.SessionID).Turns[0].ProviderID != "" })
	current := f.snapshot(sent.SessionID)
	if current.Session.AccessMode != provider.AccessFull || current.Turns[0].AccessMode != provider.AccessFull {
		t.Fatal("first task used a stale access selection")
	}
	assertNativeAccess(t, f, "thread/start", "on-request", "workspace-write")
	assertNativeAccess(t, f, "turn/start", "never", "dangerFullAccess")
}

func TestSessionAccessAppliesToNextTurnAndPersistsAcrossRestart(t *testing.T) {
	f := setup(t)
	id := f.session()
	assertNativeAccess(t, f, "thread/start", "on-request", "workspace-write")
	first := f.submit(id, "access-first", "Inspect the workspace").Session.ActiveTurn
	before := f.snapshot(id)
	if before.Turns[0].AccessMode != provider.AccessWorkspace {
		t.Fatal("existing sessions did not default to workspace access")
	}
	f.native.send("item/commandExecution/requestApproval", map[string]any{"threadId": before.Session.ProviderID, "turnId": before.Turns[0].ProviderID, "itemId": "command", "command": "fixture command"}, "access-pending")
	eventually(t, func() bool { return len(f.snapshot(id).Interactions) == 1 })
	f.call("PATCH", "/sessions/"+id, map[string]any{"accessMode": "full"}, 200, nil)
	current := f.snapshot(id)
	if current.Session.AccessMode != provider.AccessFull || current.Turns[0].AccessMode != provider.AccessWorkspace || current.Session.ActiveTurn != first || current.Interactions[0].State != "pending" {
		t.Fatal("saving access changed an admitted turn or answered its approval")
	}
	assertNativeAccess(t, f, "turn/start", "on-request", "workspaceWrite")
	select {
	case <-f.native.replies:
		t.Fatal("permission selection replied to a native approval")
	default:
	}
	f.native.finish(before.Session.ProviderID, before.Turns[0].ProviderID, "completed", "Fixture inspection complete")
	eventually(t, func() bool { return f.snapshot(id).Session.State == "idle" })
	f.submit(id, "access-second", "Run with full access")
	assertNativeAccess(t, f, "turn/start", "never", "dangerFullAccess")
	current = f.snapshot(id)
	if current.Turns[1].AccessMode != provider.AccessFull {
		t.Fatal("full access was not captured by the next turn")
	}
	f.native.finish(current.Session.ProviderID, current.Turns[1].ProviderID, "completed", "Fixture full task complete")
	eventually(t, func() bool { return f.snapshot(id).Session.State == "idle" })
	f.stop()
	f.start()
	f.waitReady()
	eventually(t, func() bool { return f.snapshot(id).Session.State == "idle" })
	if f.snapshot(id).Session.AccessMode != provider.AccessFull {
		t.Fatal("restart lost the saved access mode")
	}
	assertNativeAccess(t, f, "thread/resume", "never", "danger-full-access")
	f.call("PATCH", "/sessions/"+id, map[string]any{"accessMode": "workspace"}, 200, nil)
	f.submit(id, "access-third", "Return to workspace access")
	assertNativeAccess(t, f, "turn/start", "on-request", "workspaceWrite")
	if f.snapshot(id).Turns[2].AccessMode != provider.AccessWorkspace {
		t.Fatal("restricted access was not restored")
	}
}

func TestRequirementAccessIsLazyScopedAndSharedWithItsConversation(t *testing.T) {
	f := setup(t)
	id := requirementForChat(t, f)
	route := "/requirements/" + id + "/conversation/access"
	for _, input := range []map[string]any{{}, {"accessMode": ""}, {"accessMode": "unknown"}} {
		f.call("PATCH", route, input, 400, nil)
	}
	f.call("PATCH", "/requirements/missing/conversation/access", map[string]any{"accessMode": "full"}, 404, nil)
	f.call("PATCH", route, map[string]any{"accessMode": "full"}, 200, nil)
	var snapshot struct {
		Sessions     []model.Session
		Requirements []model.Requirement
	}
	f.call("GET", "/bootstrap", nil, 200, &snapshot)
	if len(snapshot.Sessions) != 0 {
		t.Fatal("changing requirement access created a conversation")
	}
	f.stop()
	f.start()
	f.waitReady()
	f.call("GET", "/bootstrap", nil, 200, &snapshot)
	found := false
	for _, requirement := range snapshot.Requirements {
		if requirement.ID == id {
			found = requirement.ConversationAccessMode == provider.AccessFull
		}
	}
	if !found {
		t.Fatal("the requirement preference did not survive restart")
	}
	var feature struct{ ID string }
	f.call("POST", "/sessions", map[string]any{"endpointId": f.endpoint, "requirementId": id}, 201, &feature)
	if f.snapshot(feature.ID).Session.AccessMode != provider.AccessWorkspace {
		t.Fatal("requirement conversation permissions leaked to feature sessions")
	}
	var sent struct{ SessionID, TurnID string }
	f.call("POST", "/requirements/"+id+"/conversation/turns", map[string]any{"text": "Align design", "clientRequestId": "access-chat", "section": "designDocuments"}, 202, &sent)
	eventually(t, func() bool { return f.snapshot(sent.SessionID).Turns[0].ProviderID != "" })
	assertNativeAccess(t, f, "thread/start", "never", "danger-full-access")
	assertNativeAccess(t, f, "turn/start", "never", "dangerFullAccess")
	f.call("PATCH", route, map[string]any{"accessMode": "workspace"}, 200, nil)
	conversation := f.snapshot(sent.SessionID)
	if conversation.Session.AccessMode != provider.AccessWorkspace || conversation.Turns[0].AccessMode != provider.AccessFull {
		t.Fatal("requirement access was not saved independently of the active turn")
	}
	f.call("PATCH", "/sessions/"+sent.SessionID, map[string]any{"accessMode": "full"}, 200, nil)
	f.call("GET", "/bootstrap", nil, 200, &snapshot)
	for _, requirement := range snapshot.Requirements {
		if requirement.ID == id && requirement.ConversationAccessMode != provider.AccessFull {
			t.Fatal("session access did not update its owning conversation preference")
		}
	}
	f.call("PATCH", "/sessions/"+feature.ID, map[string]any{"accessMode": "invalid"}, 400, nil)
	if f.snapshot(feature.ID).Session.AccessMode != provider.AccessWorkspace {
		t.Fatal("invalid access mode changed the feature session")
	}
}
