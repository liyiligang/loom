package httpapi_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"context"
	"github.com/liyiligang/loom/internal/model"
	"github.com/liyiligang/loom/internal/storage"
	"github.com/liyiligang/loom/internal/workspace"
)

func TestExistingRequirementsChangeDefaultBranchAndRetainSessionWorkspaces(t *testing.T) {
	f, _ := localFixture(t, true, nil)
	_ = f.localReady()
	f.call("POST", "/local/confirm", nil, 200, nil)
	f.waitReady()
	f.endpoint = "local-codex"
	directory := gitProject(t)
	var legacy, first, second, next struct{ ID string }
	f.call("POST", "/sessions", map[string]any{"endpointId": f.endpoint, "cwd": directory}, 201, &legacy)
	task := f.submit(legacy.ID, "legacy", "Existing development")
	f.native.finish(task.Session.ProviderID, task.Turns[0].ProviderID, "completed", "Saved history")
	eventually(t, func() bool { return f.snapshot(legacy.ID).Session.State == "idle" })
	before := f.snapshot(legacy.ID)
	id := before.Session.RequirementID
	f.call("PATCH", "/requirements/"+id, map[string]any{"branch": "feature/first"}, 200, nil)
	a := f.waitWorkspace(id, "ready")
	if !a.DefaultName || a.Name != "General" || f.snapshot(legacy.ID).Session.Workspace != nil {
		t.Fatal("binding changed legacy grouping or repointed its session")
	}
	f.call("PATCH", "/sessions/"+legacy.ID, map[string]any{"title": "Legacy", "requirementId": id}, 200, nil)
	f.call("POST", "/sessions", map[string]any{"endpointId": f.endpoint, "requirementId": id}, 201, &first)
	firstSession := f.snapshot(first.ID).Session
	if firstSession.Workspace == nil || firstSession.Cwd != a.Workspace.Directory || firstSession.Workspace.Branch != "feature/first" {
		t.Fatal("new session did not save its branch and directory")
	}
	marker := filepath.Join(a.Workspace.Directory, "local-work.txt")
	if err := os.WriteFile(marker, []byte("Uncommitted feature work"), 0644); err != nil {
		t.Fatal(err)
	}
	aTask := f.submit(first.ID, "first", "Develop first branch")
	f.call("PATCH", "/requirements/"+id, map[string]any{"name": "Development", "branch": "feature/second"}, 200, nil)
	b := f.waitWorkspace(id, "ready")
	if b.Workspace.Path == a.Workspace.Path || len(b.PreviousWorkspaces) != 1 || b.PreviousWorkspaces[0].Branch != a.Workspace.Branch {
		t.Fatal("replacement reused a path or forgot branch ownership")
	}
	f.call("POST", "/requirements", map[string]any{"projectId": a.ProjectID, "name": "Conflict", "branch": a.Workspace.Branch}, 409, nil)
	f.call("POST", "/sessions", map[string]any{"endpointId": f.endpoint, "requirementId": id}, 201, &second)
	f.native.mu.Lock()
	firstDirectory := f.native.threads[firstSession.ProviderID].Cwd
	secondDirectory := f.native.threads[f.snapshot(second.ID).Session.ProviderID].Cwd
	f.native.mu.Unlock()
	if firstDirectory != a.Workspace.Directory || secondDirectory != b.Workspace.Directory {
		t.Fatal("native sessions did not receive their selected worktree directories")
	}
	bTask := f.submit(second.ID, "second", "Develop second branch")
	rootTask := f.submit(legacy.ID, "legacy-next", "Continue legacy development")
	f.native.finish(aTask.Session.ProviderID, aTask.Turns[0].ProviderID, "completed", "First ready")
	f.native.finish(bTask.Session.ProviderID, bTask.Turns[0].ProviderID, "completed", "Second ready")
	f.native.finish(rootTask.Session.ProviderID, rootTask.Turns[len(rootTask.Turns)-1].ProviderID, "completed", "Legacy ready")
	eventually(t, func() bool {
		return f.snapshot(first.ID).Session.State == "idle" && f.snapshot(second.ID).Session.State == "idle" && f.snapshot(legacy.ID).Session.State == "idle"
	})
	f.call("PATCH", "/sessions/"+first.ID, map[string]any{"title": "First feature", "requirementId": id}, 200, nil)
	f.call("PATCH", "/requirements/"+id, map[string]any{"name": "Invalid", "branch": "bad branch"}, 400, nil)
	if f.waitWorkspace(id, "ready").Name != "Development" {
		t.Fatal("failed replacement partially changed metadata")
	}
	// Emulate a pre-upgrade bound session without its new workspace snapshot.
	beforeRestart := f.snapshot(legacy.ID)
	f.stop()
	store, err := storage.Open(filepath.Join(f.dataPath, "loom.db"))
	if err != nil {
		t.Fatal(err)
	}
	data, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, session := range data.Sessions {
		if session.ID == second.ID {
			session.Workspace = nil
			if _, err := store.Commit("session.updated", session.ID, session); err != nil {
				t.Fatal(err)
			}
		}
	}
	store.Close()
	f.start()
	f.waitReady()
	eventually(t, func() bool {
		return f.snapshot(first.ID).Session.State == "idle" && f.snapshot(second.ID).Session.State == "idle" && f.snapshot(legacy.ID).Session.State == "idle"
	})
	after := f.snapshot(legacy.ID)
	if after.Session.Cwd != before.Session.Cwd || after.Session.ProviderID != before.Session.ProviderID || after.Session.Workspace != nil || len(after.Entries) != len(beforeRestart.Entries) {
		t.Fatal("restart changed legacy execution or history")
	}
	if f.snapshot(first.ID).Session.Workspace.Branch != a.Workspace.Branch || f.snapshot(second.ID).Session.Workspace.Branch != b.Workspace.Branch {
		t.Fatal("saved session binding or upgrade backfill was lost")
	}
	continued := f.submit(first.ID, "first-next", "Continue first branch")
	f.native.finish(continued.Session.ProviderID, continued.Turns[len(continued.Turns)-1].ProviderID, "completed", "Continued")
	eventually(t, func() bool { return f.snapshot(first.ID).Session.State == "idle" })
	// A failed selection does not prevent sessions in other saved worktrees running.
	gitCommand(t, a.Workspace.Path, "switch", "-c", "feature/external-change")
	f.call("PATCH", "/requirements/"+id, map[string]any{"branch": a.Workspace.Branch}, 200, nil)
	f.waitWorkspace(id, "error")
	f.call("POST", "/sessions/"+first.ID+"/turns", map[string]any{"clientRequestId": "changed", "text": "Must not run"}, 409, nil)
	independent := f.submit(second.ID, "independent", "Continue second branch")
	f.native.finish(independent.Session.ProviderID, independent.Turns[len(independent.Turns)-1].ProviderID, "completed", "Second continued")
	eventually(t, func() bool { return f.snapshot(second.ID).Session.State == "idle" })
	gitCommand(t, a.Workspace.Path, "switch", a.Workspace.Branch)
	f.call("PATCH", "/requirements/"+id, map[string]any{"branch": a.Workspace.Branch}, 200, nil)
	reused := f.waitWorkspace(id, "ready")
	if reused.Workspace.Path != a.Workspace.Path || len(reused.PreviousWorkspaces) != 1 || reused.PreviousWorkspaces[0].Branch != b.Workspace.Branch {
		t.Fatal("switching back lost a worktree")
	}
	if raw, err := os.ReadFile(marker); err != nil || string(raw) != "Uncommitted feature work" {
		t.Fatal("switching back changed local work")
	}
	f.call("POST", "/sessions", map[string]any{"endpointId": f.endpoint, "requirementId": id}, 201, &next)
	if f.snapshot(next.ID).Session.Cwd != a.Workspace.Directory {
		t.Fatal("new sessions did not follow the selected previous branch")
	}
	f.call("PATCH", "/requirements/"+id, map[string]any{"branch": ""}, 200, nil)
	generated := f.waitWorkspace(id, "ready")
	if !strings.HasPrefix(generated.Workspace.Branch, "codex/requirement-") || generated.Workspace.Path == a.Workspace.Path || len(generated.PreviousWorkspaces) != 2 {
		t.Fatal("empty replacement did not create a fresh branch")
	}
	if gitCommand(t, directory, "branch", "--show-current") != "main" {
		t.Fatal("source checkout changed")
	}
}

func TestRequirementBranchChangeWaitsForNativeSessionCreation(t *testing.T) {
	f, _ := localFixture(t, true, nil)
	_ = f.localReady()
	f.call("POST", "/local/confirm", nil, 200, nil)
	f.waitReady()
	directory := gitProject(t)
	var project, requirement struct{ ID string }
	f.call("POST", "/projects", map[string]any{"endpointId": "local-codex", "cwd": directory}, 201, &project)
	f.call("POST", "/requirements", map[string]any{"projectId": project.ID, "name": "Creation", "branch": "feature/create"}, 201, &requirement)
	a := f.waitWorkspace(requirement.ID, "ready")
	started, release := make(chan struct{}, 1), make(chan struct{})
	f.native.mu.Lock()
	f.native.createStarted, f.native.createRelease = started, release
	f.native.mu.Unlock()
	result := make(chan struct{ ID string }, 1)
	go func() {
		var session struct{ ID string }
		f.call("POST", "/sessions", map[string]any{"endpointId": "local-codex", "requirementId": requirement.ID}, 201, &session)
		result <- session
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		close(release)
		t.Fatal("native creation did not start")
	}
	f.call("PATCH", "/requirements/"+requirement.ID, map[string]any{"name": "Must stay", "branch": "feature/change"}, 409, nil)
	close(release)
	session := <-result
	if f.snapshot(session.ID).Session.Workspace.Branch != a.Workspace.Branch || f.waitWorkspace(requirement.ID, "ready").Name != "Creation" {
		t.Fatal("creation race changed the branch snapshot or requirement")
	}
	f.call("PATCH", "/requirements/"+requirement.ID, map[string]any{"branch": "feature/change"}, 200, nil)
	f.waitWorkspace(requirement.ID, "ready")
}

func gitCommand(t *testing.T, directory string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", directory}, args...)...)
	raw, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, raw)
	}
	return strings.TrimSpace(string(raw))
}

func gitProject(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("Git unavailable")
	}
	directory := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(directory, 0755); err != nil {
		t.Fatal(err)
	}
	gitCommand(t, directory, "init", "-b", "main")
	gitCommand(t, directory, "config", "user.name", "Loom Test")
	gitCommand(t, directory, "config", "user.email", "loom@example.test")
	installSkill(t, directory, "example", "Project guidance")
	gitCommand(t, directory, "add", ".")
	gitCommand(t, directory, "commit", "-m", "Initial")
	return directory
}

func (f *fixture) waitWorkspace(id, state string) model.Requirement {
	f.t.Helper()
	var result model.Requirement
	eventually(f.t, func() bool {
		var groups navigation
		f.call("GET", "/bootstrap", nil, 200, &groups)
		for _, value := range groups.Requirements {
			if value.ID == id {
				result = value
				return result.Workspace != nil && result.Workspace.State == state
			}
		}
		return false
	})
	return result
}

func TestRequirementWorktreesInheritDirectoriesAndSerializeWriters(t *testing.T) {
	f, _ := localFixture(t, true, nil)
	_ = f.localReady()
	f.call("POST", "/local/confirm", nil, 200, nil)
	f.waitReady()
	f.endpoint = "local-codex"
	directory := gitProject(t)
	var project, first, second struct{ ID string }
	f.call("POST", "/projects", map[string]any{"endpointId": f.endpoint, "cwd": directory}, 201, &project)
	var repository workspace.Repository
	f.call("GET", "/projects/"+project.ID+"/git", nil, 200, &repository)
	if repository.CurrentBranch != "main" || len(repository.Branches) != 1 {
		t.Fatalf("Git inspection: %+v", repository)
	}
	f.call("POST", "/requirements", map[string]any{"projectId": project.ID, "name": "Account", "branch": "feature/account"}, 201, &first)
	f.call("POST", "/requirements", map[string]any{"projectId": project.ID, "name": "Search", "branch": ""}, 201, &second)
	a, b := f.waitWorkspace(first.ID, "ready"), f.waitWorkspace(second.ID, "ready")
	if a.Workspace.Path == b.Workspace.Path || !strings.HasPrefix(b.Workspace.Branch, "codex/requirement-") {
		t.Fatal("requirements did not get independent branches and paths")
	}
	f.call("POST", "/requirements", map[string]any{"projectId": project.ID, "name": "Duplicate", "branch": a.Workspace.Branch}, 409, nil)
	f.call("POST", "/requirements", map[string]any{"projectId": project.ID, "name": "Main", "branch": "main"}, 400, nil)
	var s1, s2, s3 struct{ ID string }
	for _, target := range []struct {
		requirement string
		result      any
	}{{first.ID, &s1}, {first.ID, &s2}, {second.ID, &s3}} {
		f.call("POST", "/sessions", map[string]any{"endpointId": f.endpoint, "requirementId": target.requirement}, 201, target.result)
	}
	f.call("POST", "/sessions", map[string]any{"endpointId": f.endpoint, "requirementId": first.ID, "cwd": directory}, 400, nil)
	if f.snapshot(s1.ID).Session.Cwd != a.Workspace.Directory || f.snapshot(s2.ID).Session.Cwd != a.Workspace.Directory || f.snapshot(s3.ID).Session.Cwd != b.Workspace.Directory {
		t.Fatal("session directory did not inherit its requirement worktree")
	}
	var catalog struct {
		Directory string
		Skills    []any
	}
	f.call("GET", "/projects/"+project.ID+"/skills?sessionId="+s1.ID, nil, 200, &catalog)
	if catalog.Directory != a.Workspace.Directory || len(catalog.Skills) != 1 {
		t.Fatal("skills did not use the prepared worktree")
	}
	aTask := f.submit(s1.ID, "account-task", "Build account")
	bTask := f.submit(s3.ID, "search-task", "Build search")
	f.call("POST", "/sessions/"+s2.ID+"/turns", map[string]any{"clientRequestId": "blocked", "text": "Other writer"}, 409, nil)
	f.call("POST", "/sessions/"+s1.ID+"/turns", map[string]any{"clientRequestId": "account-task", "text": "Build account"}, 202, nil)
	f.native.finish(aTask.Session.ProviderID, aTask.Turns[0].ProviderID, "completed", "Account ready")
	f.native.finish(bTask.Session.ProviderID, bTask.Turns[0].ProviderID, "completed", "Search ready")
	eventually(t, func() bool {
		return f.snapshot(s1.ID).Session.State == "idle" && f.snapshot(s3.ID).Session.State == "idle"
	})
	otherTask := f.submit(s2.ID, "next-writer", "Continue account")
	f.native.finish(otherTask.Session.ProviderID, otherTask.Turns[0].ProviderID, "completed", "Account continued")
	eventually(t, func() bool { return f.snapshot(s2.ID).Session.State == "idle" })
	f.call("PATCH", "/sessions/"+s1.ID, map[string]any{"requirementId": second.ID}, 400, nil)
	f.call("PATCH", "/requirements/"+first.ID, map[string]any{"name": "Account settings"}, 200, nil)
	before := f.snapshot(s1.ID)
	f.stop()
	f.start()
	f.waitReady()
	restored := f.waitWorkspace(first.ID, "ready")
	if restored.Name != "Account settings" || *restored.Workspace != *a.Workspace {
		t.Fatal("binding did not persist")
	}
	eventually(t, func() bool { return f.snapshot(s1.ID).Session.State == "idle" })
	after := f.snapshot(s1.ID)
	if after.Session.ProviderID != before.Session.ProviderID || after.Session.Cwd != before.Session.Cwd || len(after.Entries) != len(before.Entries) {
		t.Fatal("restart changed session identity or history")
	}
	if gitCommand(t, directory, "branch", "--show-current") != "main" {
		t.Fatal("source branch changed")
	}
	gitCommand(t, a.Workspace.Path, "switch", "-c", "feature/outside-change")
	f.call("POST", "/sessions/"+s1.ID+"/turns", map[string]any{"clientRequestId": "wrong-branch", "text": "Must not run"}, 409, nil)
}

func TestWorkspaceBindingRetriesAndRecoversPersistedIntent(t *testing.T) {
	f, _ := localFixture(t, true, nil)
	_ = f.localReady()
	f.call("POST", "/local/confirm", nil, 200, nil)
	f.waitReady()
	directory := gitProject(t)
	var project, requirement struct{ ID string }
	f.call("POST", "/projects", map[string]any{"endpointId": "local-codex", "cwd": directory}, 201, &project)
	f.call("POST", "/requirements", map[string]any{"projectId": project.ID, "name": "Recovery"}, 201, &requirement)
	plan, err := workspace.Plan(context.Background(), directory, "feature/retry", "", requirement.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(plan.Path, 0755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(plan.Path, "keep.txt")
	if err := os.WriteFile(marker, []byte("Keep"), 0644); err != nil {
		t.Fatal(err)
	}
	f.call("PATCH", "/requirements/"+requirement.ID, map[string]any{"branch": "feature/retry"}, 200, nil)
	failed := f.waitWorkspace(requirement.ID, "error")
	if failed.Workspace.Error == "" {
		t.Fatal("preparation error was hidden")
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatal("occupied files removed")
	}
	f.call("POST", "/sessions", map[string]any{"endpointId": "local-codex", "requirementId": requirement.ID}, 409, nil)
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(plan.Path); err != nil {
		t.Fatal(err)
	}
	f.call("PATCH", "/requirements/"+requirement.ID, map[string]any{"branch": "feature/retry"}, 200, nil)
	ready := f.waitWorkspace(requirement.ID, "ready")
	f.stop()
	store, err := storage.Open(filepath.Join(f.dataPath, "loom.db"))
	if err != nil {
		t.Fatal(err)
	}
	ready.Workspace.State = "preparing"
	if _, err := store.Commit("navigation.updated", requirement.ID, &ready); err != nil {
		t.Fatal(err)
	}
	store.Close()
	f.start()
	f.waitReady()
	recovered := f.waitWorkspace(requirement.ID, "ready")
	if recovered.Workspace.Path != ready.Workspace.Path {
		t.Fatal("recovery created another worktree")
	}
	f.native.mu.Lock()
	defer f.native.mu.Unlock()
	if len(f.native.threads) != 0 || f.native.starts != 0 {
		t.Fatal("worktree preparation started an agent task or session")
	}
}

func TestWorktreesRejectRemoteAndNonGitProjects(t *testing.T) {
	remote := setup(t)
	var project struct{ ID string }
	remote.call("POST", "/projects", map[string]any{"endpointId": remote.endpoint, "cwd": "/remote/project"}, 201, &project)
	remote.call("GET", "/projects/"+project.ID+"/git", nil, 409, nil)
	remote.call("POST", "/requirements", map[string]any{"projectId": project.ID, "name": "Remote", "branch": "feature/remote"}, 409, nil)
	f, _ := localFixture(t, true, nil)
	_ = f.localReady()
	f.call("POST", "/local/confirm", nil, 200, nil)
	f.waitReady()
	plain := t.TempDir()
	f.call("POST", "/projects", map[string]any{"endpointId": "local-codex", "cwd": plain}, 201, &project)
	f.call("GET", "/projects/"+project.ID+"/git", nil, 400, nil)
	f.call("POST", "/requirements", map[string]any{"projectId": project.ID, "name": "Plain", "branch": "feature/plain"}, 400, nil)

}

func TestUnrestorableEmptySessionDoesNotOccupyTheWorktree(t *testing.T) {
	f, _ := localFixture(t, true, nil)
	_ = f.localReady()
	f.call("POST", "/local/confirm", nil, 200, nil)
	f.waitReady()
	directory := gitProject(t)
	var project, requirement, empty, next struct{ ID string }
	f.call("POST", "/projects", map[string]any{"endpointId": "local-codex", "cwd": directory}, 201, &project)
	f.call("POST", "/requirements", map[string]any{"projectId": project.ID, "name": "Empty recovery", "branch": "feature/empty"}, 201, &requirement)
	f.waitWorkspace(requirement.ID, "ready")
	f.call("POST", "/sessions", map[string]any{"endpointId": "local-codex", "requirementId": requirement.ID}, 201, &empty)
	f.stop()
	f.start()
	f.waitReady()
	eventually(t, func() bool { return f.snapshot(empty.ID).Session.Error != "" })
	f.call("POST", "/sessions", map[string]any{"endpointId": "local-codex", "requirementId": requirement.ID}, 201, &next)
	var state struct{ Writers []struct{ SessionID string } }
	f.call("GET", "/bootstrap", nil, 200, &state)
	if len(state.Writers) != 0 {
		t.Fatal("an empty conversation reserved a writer")
	}
	f.call("POST", "/sessions/"+next.ID+"/turns", map[string]any{"clientRequestId": "new-session", "text": "Start feature"}, 202, nil)
	f.call("GET", "/bootstrap", nil, 200, &state)
	if len(state.Writers) != 1 || state.Writers[0].SessionID != next.ID {
		t.Fatal("writer admission was not published")
	}
}
