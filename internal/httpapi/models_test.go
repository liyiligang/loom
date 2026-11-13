package httpapi_test

import (
	"encoding/json"
	"strings"
	"testing"
)

func assertNativeModel(t *testing.T, f *fixture, method, want string) {
	t.Helper()
	f.native.mu.Lock()
	defer f.native.mu.Unlock()
	for i := len(f.native.requests) - 1; i >= 0; i-- {
		request := f.native.requests[i]
		if request.Method != method {
			continue
		}
		var params struct {
			Model string `json:"model"`
		}
		if err := json.Unmarshal(request.Params, &params); err != nil {
			t.Fatal(err)
		}
		if params.Model != want {
			t.Fatalf("%s model = %q, want %q", method, params.Model, want)
		}
		return
	}
	t.Fatalf("missing %s", method)
}

func projectForRequirement(t *testing.T, f *fixture, id string) string {
	t.Helper()
	var groups navigation
	f.call("GET", "/bootstrap", nil, 200, &groups)
	for _, requirement := range groups.Requirements {
		if requirement.ID == id {
			return requirement.ProjectID
		}
	}
	t.Fatal("requirement missing")
	return ""
}

func finishModelTurn(t *testing.T, f *fixture, id string) {
	t.Helper()
	current := f.snapshot(id)
	turn := current.Turns[len(current.Turns)-1]
	f.native.finish(current.Session.ProviderID, turn.ProviderID, "completed", "Fixture model task complete")
	eventually(t, func() bool { return f.snapshot(id).Session.State == "idle" })
}

func TestScopedModelsReachExistingConversationsAndResetToDefaults(t *testing.T) {
	f := setup(t)
	id := requirementForChat(t, f)
	projectID := projectForRequirement(t, f, id)
	projectRoute, requirementRoute := "/projects/"+projectID+"/models", "/requirements/"+id+"/models"
	f.call("PATCH", projectRoute, map[string]any{"models": map[string]string{"codex": "project-model", "claude": "opus"}}, 200, nil)
	f.call("PATCH", requirementRoute, map[string]any{"models": map[string]string{"claude": "sonnet"}}, 200, nil)
	var feature struct{ ID string }
	f.call("POST", "/sessions", map[string]any{"endpointId": f.endpoint, "requirementId": id}, 201, &feature)
	assertNativeModel(t, f, "thread/start", "project-model")
	var chat struct{ SessionID, TurnID string }
	f.call("POST", "/requirements/"+id+"/conversation/turns", map[string]any{"text": "Align design", "clientRequestId": "models-chat", "section": "designDocuments"}, 202, &chat)
	eventually(t, func() bool { return f.snapshot(chat.SessionID).Turns[0].ProviderID != "" })
	assertNativeModel(t, f, "turn/start", "project-model")
	f.call("PATCH", requirementRoute, map[string]any{"models": map[string]string{"codex": "requirement-model"}}, 200, nil)
	current := f.snapshot(chat.SessionID)
	if *current.Turns[0].Model != "project-model" || current.Session.Model != "project-model" {
		t.Fatal("preference changed the active task")
	}
	finishModelTurn(t, f, chat.SessionID)
	f.submit(feature.ID, "models-feature", "Implement the requirement")
	assertNativeModel(t, f, "turn/start", "requirement-model")
	finishModelTurn(t, f, feature.ID)
	f.call("PATCH", projectRoute, map[string]any{"models": map[string]string{"codex": "updated-project"}}, 200, nil)
	f.submit(chat.SessionID, "models-override", "Continue design")
	assertNativeModel(t, f, "turn/start", "requirement-model")
	finishModelTurn(t, f, chat.SessionID)
	f.call("PATCH", requirementRoute, map[string]any{"models": map[string]string{}}, 200, nil)
	f.submit(chat.SessionID, "models-inherit", "Inherit the project model")
	assertNativeModel(t, f, "turn/start", "updated-project")
	finishModelTurn(t, f, chat.SessionID)
	f.call("PATCH", requirementRoute, map[string]any{"models": map[string]string{"codex": ""}}, 200, nil)
	f.submit(chat.SessionID, "models-default", "Use the agent default")
	assertNativeModel(t, f, "turn/start", "test-model")
	finishModelTurn(t, f, chat.SessionID)
	f.call("PATCH", projectRoute, map[string]any{"models": map[string]string{}}, 200, nil)
	f.call("PATCH", requirementRoute, map[string]any{"models": map[string]string{}}, 200, nil)
	f.submit(feature.ID, "models-cleared", "Clear all scope overrides")
	assertNativeModel(t, f, "turn/start", "test-model")
}

func TestScopedModelsPersistAndRestoreTheAdmittedModel(t *testing.T) {
	f := setup(t)
	id := requirementForChat(t, f)
	projectID := projectForRequirement(t, f, id)
	f.call("PATCH", "/projects/"+projectID+"/models", map[string]any{"models": map[string]string{"codex": "project-model", "claude": "opus"}}, 200, nil)
	var feature struct{ ID string }
	f.call("POST", "/sessions", map[string]any{"endpointId": f.endpoint, "requirementId": id}, 201, &feature)
	f.submit(feature.ID, "models-running", "Run the model fixture")
	f.call("PATCH", "/requirements/"+id+"/models", map[string]any{"models": map[string]string{"codex": "next-model", "claude": "haiku"}}, 200, nil)
	f.stop()
	f.start()
	f.waitReady()
	eventually(t, func() bool { return f.snapshot(feature.ID).Session.State == "running" })
	assertNativeModel(t, f, "thread/resume", "project-model")
	current := f.snapshot(feature.ID)
	if current.Turns[0].Model == nil || *current.Turns[0].Model != "project-model" {
		t.Fatal("restart lost the admitted model")
	}
	var groups navigation
	f.call("GET", "/bootstrap", nil, 200, &groups)
	for _, project := range groups.Projects {
		if project.ID == projectID && project.Models["claude"] != "opus" {
			t.Fatal("project preference lost")
		}
	}
	for _, requirement := range groups.Requirements {
		if requirement.ID == id && requirement.Models["claude"] != "haiku" {
			t.Fatal("requirement preference lost")
		}
	}
	finishModelTurn(t, f, feature.ID)
	f.submit(feature.ID, "models-after-restart", "Use the new preference")
	assertNativeModel(t, f, "turn/start", "next-model")
}

func TestScopedModelChangeDuringLazyCreationReachesFirstTurn(t *testing.T) {
	f := setup(t)
	id := requirementForChat(t, f)
	f.call("PATCH", "/requirements/"+id+"/models", map[string]any{"models": map[string]string{"codex": "creation-model"}}, 200, nil)
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
		status, body, err := f.request("POST", "/requirements/"+id+"/conversation/turns", map[string]any{"text": "Align design", "clientRequestId": "model-creation", "section": "designDocuments"})
		done <- struct {
			status int
			body   []byte
			err    error
		}{status, body, err}
	}()
	<-started
	f.call("PATCH", "/requirements/"+id+"/models", map[string]any{"models": map[string]string{"codex": "first-turn-model"}}, 200, nil)
	close(release)
	result := <-done
	if result.err != nil || result.status != 202 {
		t.Fatalf("submission: %d %v %s", result.status, result.err, result.body)
	}
	var sent struct{ SessionID, TurnID string }
	if err := json.Unmarshal(result.body, &sent); err != nil {
		t.Fatal(err)
	}
	eventually(t, func() bool { return f.snapshot(sent.SessionID).Turns[0].ProviderID != "" })
	assertNativeModel(t, f, "thread/start", "creation-model")
	assertNativeModel(t, f, "turn/start", "first-turn-model")
}

func TestScopedModelsValidateWithoutCreatingNativeSessions(t *testing.T) {
	f := setup(t)
	id := requirementForChat(t, f)
	projectID := projectForRequirement(t, f, id)
	for _, route := range []string{"/projects/" + projectID + "/models", "/requirements/" + id + "/models"} {
		for _, input := range []any{map[string]any{}, map[string]any{"models": nil}, map[string]any{"models": map[string]string{"unknown": "x"}}, map[string]any{"models": map[string]string{"codex": "x y"}}, map[string]any{"models": map[string]string{"codex": "x\x00y"}}, map[string]any{"models": map[string]string{"claude": strings.Repeat("x", 121)}}} {
			f.call("PATCH", route, input, 400, nil)
		}
		f.call("PATCH", route, map[string]any{"models": map[string]string{"codex": "  custom-model  ", "claude": "opus"}}, 200, nil)
	}
	f.call("PATCH", "/projects/missing/models", map[string]any{"models": map[string]string{}}, 404, nil)
	f.call("PATCH", "/requirements/missing/models", map[string]any{"models": map[string]string{}}, 404, nil)
	var groups navigation
	f.call("GET", "/bootstrap", nil, 200, &groups)
	if len(groups.Sessions) != 0 || groups.Requirements[len(groups.Requirements)-1].Models["codex"] != "custom-model" {
		t.Fatal("settings created a conversation or failed to normalize")
	}

}

func TestPreparationStagesUseTheLatestScopedModel(t *testing.T) {
	f, id, directory := materialFixture(t)
	projectID := projectForRequirement(t, f, id)
	f.call("PATCH", "/projects/"+projectID+"/models", map[string]any{"models": map[string]string{"codex": "planner-model"}}, 200, nil)
	f.call("POST", workflowRoute(id)+"/actions", map[string]any{"action": "start", "brief": "Build a sign-in flow.", "clientRequestId": "models-preparation"}, 200, nil)
	w := waitWorkflow(f, id, "requirements", "running")
	eventually(t, func() bool { return f.snapshot(w.SessionID).Turns[0].ProviderID != "" })
	assertNativeModel(t, f, "turn/start", "planner-model")
	f.call("PATCH", "/requirements/"+id+"/models", map[string]any{"models": map[string]string{"codex": "design-model"}}, 200, nil)
	writeWorkflowFile(t, directory, id, "documents/requirements.md", "# Sign in\n\nAC1: A valid account can sign in.\n")
	stageReceipt(t, directory, id, w, nil, nil)
	finishWorkflowTurn(f, w)
	w = waitWorkflow(f, id, "design", "running")
	eventually(t, func() bool {
		return len(f.snapshot(w.SessionID).Turns) == 2 && f.snapshot(w.SessionID).Turns[1].ProviderID != ""
	})
	assertNativeModel(t, f, "turn/start", "design-model")
	current := f.snapshot(w.SessionID)
	if *current.Turns[0].Model != "planner-model" || *current.Turns[1].Model != "design-model" {
		t.Fatal("preparation lost stage model choices")
	}
}
