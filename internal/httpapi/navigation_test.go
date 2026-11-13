package httpapi_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/liyiligang/loom/internal/model"
	"github.com/liyiligang/loom/internal/storage"
)

type navigation struct {
	Projects     []model.Project
	Requirements []model.Requirement
	Sessions     []model.Session
}

func TestHierarchyPersistsAndMovesOnlySessionMetadata(t *testing.T) {
	f := setup(t)
	var project, requirement, session, second struct{ ID string }
	f.call("POST", "/projects", map[string]any{"endpointId": f.endpoint, "cwd": "/target/project", "name": "App"}, 201, &project)
	f.call("POST", "/requirements", map[string]any{"projectId": project.ID, "name": "Account sign-in"}, 201, &requirement)
	f.call("POST", "/sessions", map[string]any{"endpointId": f.endpoint, "requirementId": requirement.ID, "title": "Sign-in form"}, 201, &session)
	f.call("POST", "/sessions", map[string]any{"endpointId": f.endpoint, "requirementId": requirement.ID, "title": "Authentication API", "cwd": "/target/feature-worktree"}, 201, &second)
	var before detail
	f.call("GET", "/sessions/"+session.ID, nil, 200, &before)
	if before.Session.Cwd != "/target/project" || before.Session.RequirementID != requirement.ID {
		t.Fatal("session did not inherit its project directory and requirement")
	}
	f.call("POST", "/requirements", map[string]any{"projectId": project.ID, "name": "Profile settings"}, 201, &second)
	var turn struct{ ID string }
	f.call("POST", "/sessions/"+session.ID+"/turns", map[string]any{"text": "Build sign-in", "clientRequestId": "hierarchy-task"}, 202, &turn)
	eventually(t, func() bool {
		var current detail
		f.call("GET", "/sessions/"+session.ID, nil, 200, &current)
		return len(current.Turns) == 1 && current.Turns[0].ProviderID != ""
	})
	f.call("PATCH", "/sessions/"+session.ID, map[string]any{"requirementId": second.ID}, 200, nil)
	var moved detail
	f.call("GET", "/sessions/"+session.ID, nil, 200, &moved)
	if moved.Session.Cwd != before.Session.Cwd || moved.Session.ProviderID != before.Session.ProviderID || moved.Session.RequirementID != second.ID || moved.Session.ActiveTurn != turn.ID || moved.Session.Title != "Sign-in form" {
		t.Fatal("moving metadata changed native execution or title")
	}
	f.native.finish(moved.Session.ProviderID, moved.Turns[0].ProviderID, "completed", "Sign-in is ready")
	eventually(t, func() bool {
		f.call("GET", "/sessions/"+session.ID, nil, 200, &moved)
		return moved.Session.State == "idle"
	})
	f.call("PATCH", "/projects/"+project.ID, map[string]any{"name": "Application"}, 200, nil)
	f.call("PATCH", "/requirements/"+second.ID, map[string]any{"name": "Account profile"}, 200, nil)
	var otherProject struct{ ID string }
	f.call("POST", "/projects", map[string]any{"endpointId": f.endpoint, "cwd": "D:\\Projects\\other", "name": "Other app"}, 201, &otherProject)
	f.call("POST", "/projects", map[string]any{"endpointId": f.endpoint, "cwd": "/target/project"}, 409, nil)
	var groups navigation
	f.call("GET", "/bootstrap", nil, 200, &groups)
	if len(groups.Projects) != 2 || len(groups.Requirements) != 4 || len(groups.Sessions) != 2 || groups.Projects[0].Name != "Application" {
		t.Fatalf("incomplete hierarchy: %+v", groups)
	}
	f.stop()
	f.start()
	f.waitReady()
	var restarted navigation
	f.call("GET", "/bootstrap", nil, 200, &restarted)
	if len(restarted.Projects) != 2 || len(restarted.Requirements) != 4 || restarted.Requirements[2].Name != "Account profile" {
		t.Fatal("group metadata did not survive restart")
	}
	var restored detail
	f.call("GET", "/sessions/"+session.ID, nil, 200, &restored)
	if restored.Session.RequirementID != second.ID || restored.Session.ProviderID != before.Session.ProviderID || len(restored.Entries) != len(moved.Entries) || restored.Entries[len(restored.Entries)-1].Text != "Sign-in is ready" {
		t.Fatal("membership or history changed across restart")
	}
}

func TestHierarchyRejectsInvalidScopeBeforeCreatingNativeSession(t *testing.T) {
	f := setup(t)
	id := f.session()
	var other struct{ ID string }
	f.call("POST", "/endpoints", map[string]any{"name": "Other machine", "address": "ws" + strings.TrimPrefix(f.native.server.URL, "http") + "/other", "token": "test-service-token"}, 202, &other)
	var project, requirement struct{ ID string }
	f.call("POST", "/projects", map[string]any{"endpointId": other.ID, "cwd": "/target/project"}, 201, &project)
	f.call("POST", "/requirements", map[string]any{"projectId": project.ID, "name": "Remote work"}, 201, &requirement)
	f.call("POST", "/sessions", map[string]any{"endpointId": f.endpoint, "requirementId": requirement.ID}, 400, nil)
	f.call("PATCH", "/sessions/"+id, map[string]any{"title": "Must not change", "requirementId": requirement.ID}, 400, nil)
	f.call("PATCH", "/sessions/"+id, map[string]any{}, 400, nil)
	f.call("POST", "/sessions", map[string]any{"endpointId": f.endpoint, "requirementId": "missing"}, 404, nil)
	f.call("POST", "/requirements", map[string]any{"projectId": "missing", "name": "Work"}, 404, nil)
	f.call("POST", "/requirements", map[string]any{"projectId": project.ID, "name": "  "}, 400, nil)
	var current detail
	f.call("GET", "/sessions/"+id, nil, 200, &current)
	if current.Session.Title != "New session" {
		t.Fatal("rejected move partially renamed the session")
	}
	f.native.mu.Lock()
	defer f.native.mu.Unlock()
	if len(f.native.threads) != 1 {
		t.Fatal("invalid requirement created a native session")
	}
}

func TestLegacySessionsBackfillOnceWithoutLosingHistory(t *testing.T) {
	dataPath := t.TempDir()
	store, err := storage.Open(filepath.Join(dataPath, "loom.db"))
	if err != nil {
		t.Fatal(err)
	}
	endpoint := &model.Endpoint{ID: "old-remote", Provider: "codex", Address: "ws://127.0.0.1:1/"}
	first := &model.Session{ID: "s1", EndpointID: endpoint.ID, ProviderID: "thread-1", Title: "Old feature", Cwd: "/old/project", State: "idle"}
	second := &model.Session{ID: "s2", EndpointID: endpoint.ID, ProviderID: "thread-2", Title: "Other feature", Cwd: "/old/project", State: "idle"}
	turn := &model.Turn{ID: "t1", SessionID: first.ID, ClientRequestID: "old-request", State: "completed"}
	entry := &model.Entry{ID: "e1", SessionID: first.ID, TurnID: turn.ID, Text: "Existing history", Status: "completed"}
	if _, err = store.Commit("state.updated", "", endpoint, first, second, turn, entry); err != nil {
		t.Fatal(err)
	}
	store.Close()
	f := &fixture{t: t, dataPath: dataPath, manager: &localManager{}}
	f.start()
	t.Cleanup(f.stop)
	var initial navigation
	f.call("GET", "/bootstrap", nil, 200, &initial)
	if len(initial.Projects) != 1 || len(initial.Requirements) != 1 || initial.Sessions[0].RequirementID != initial.Sessions[1].RequirementID || !initial.Requirements[0].DefaultName {
		t.Fatal("legacy sessions were not grouped by target directory")
	}
	f.stop()
	f.start()
	var restarted navigation
	f.call("GET", "/bootstrap", nil, 200, &restarted)
	if len(restarted.Projects) != 1 || len(restarted.Requirements) != 1 || restarted.Projects[0].ID != initial.Projects[0].ID || restarted.Requirements[0].ID != initial.Requirements[0].ID {
		t.Fatal("restart duplicated default groups")
	}
	var history detail
	f.call("GET", "/sessions/s1", nil, 200, &history)
	if history.Session.ProviderID != "thread-1" || history.Session.Cwd != "/old/project" || history.Session.Title != "Old feature" || len(history.Entries) != 1 || history.Entries[0].Text != "Existing history" {
		t.Fatal("backfill changed legacy history")
	}
}

func TestLocalProjectUsesCanonicalDirectoryAndRequiresAvailableFolder(t *testing.T) {
	f, _ := localFixture(t, true, nil)
	_ = f.localReady()
	f.call("POST", "/local/confirm", nil, 200, nil)
	f.waitReady()
	directory := t.TempDir()
	link := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(directory, link); err != nil {
		t.Skip("symlinks unavailable")
	}
	var project struct{ ID string }
	f.call("POST", "/projects", map[string]any{"endpointId": "local-codex", "cwd": link}, 201, &project)
	f.call("POST", "/projects", map[string]any{"endpointId": "local-codex", "cwd": directory}, 409, nil)
	f.call("POST", "/projects", map[string]any{"endpointId": "local-codex", "cwd": filepath.Join(directory, "missing")}, 400, nil)
	var groups navigation
	f.call("GET", "/bootstrap", nil, 200, &groups)
	canonical, _ := filepath.EvalSymlinks(directory)
	if !groups.Projects[0].Local || groups.Projects[0].EndpointID != "" || groups.Projects[0].Cwd != canonical {
		t.Fatal("local project did not use canonical, shared machine scope")
	}
}

func TestLocalAgentsShareProjectsAndRemotePathsRemainSeparate(t *testing.T) {
	dataPath := t.TempDir()
	directory := t.TempDir()
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(directory, alias); err != nil {
		t.Skip("symlinks unavailable")
	}
	store, err := storage.Open(filepath.Join(dataPath, "loom.db"))
	if err != nil {
		t.Fatal(err)
	}
	codex := &model.Endpoint{ID: "local-codex", Local: true, Provider: "codex"}
	claude := &model.Endpoint{ID: "local-claude", Local: true, Provider: "claude"}
	remote := &model.Endpoint{ID: "remote", Provider: "codex", Address: "ws://127.0.0.1:1/"}
	values := []any{codex, claude, remote}
	for i, spec := range []struct{ endpoint, cwd string }{{codex.ID, directory}, {claude.ID, alias}, {remote.ID, directory}} {
		values = append(values, &model.Session{ID: fmt.Sprintf("session-%d", i), EndpointID: spec.endpoint, ProviderID: fmt.Sprintf("native-%d", i), Cwd: spec.cwd, State: "idle"})
	}
	if _, err = store.Commit("state.updated", "", values...); err != nil {
		t.Fatal(err)
	}
	store.Close()
	f := &fixture{t: t, dataPath: dataPath, manager: &localManager{}}
	f.start()
	t.Cleanup(f.stop)
	var groups navigation
	f.call("GET", "/bootstrap", nil, 200, &groups)
	if len(groups.Projects) != 2 || len(groups.Requirements) != 2 || groups.Sessions[0].RequirementID != groups.Sessions[1].RequirementID || groups.Sessions[2].RequirementID == groups.Sessions[0].RequirementID {
		t.Fatal("local agents did not share a project or remote target leaked into local scope")
	}
	f.call("POST", "/projects", map[string]any{"endpointId": codex.ID, "cwd": directory}, 409, nil)
}
