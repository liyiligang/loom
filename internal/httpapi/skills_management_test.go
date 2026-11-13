package httpapi_test

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/liyiligang/loom/internal/controller"
	"github.com/liyiligang/loom/internal/skills"
)

func skillManagementFixture(t *testing.T) (*fixture, string, string) {
	t.Helper()
	f, _ := localFixture(t, true, nil)
	_ = f.localReady()
	f.call("POST", "/local/confirm", nil, 200, nil)
	f.waitReady()
	f.endpoint = "local-codex"
	directory := t.TempDir()
	var project struct{ ID string }
	f.call("POST", "/projects", map[string]any{"endpointId": f.endpoint, "cwd": directory}, 201, &project)
	return f, project.ID, directory
}

func TestSkillManagementPersistsFullSourceAndRequiresDeletionConfirmation(t *testing.T) {
	f, id, directory := skillManagementFixture(t)
	route := "/projects/" + id + "/skills"
	var catalog controller.ProjectSkillCatalog
	f.call("GET", route, nil, 200, &catalog)
	source := "---\nname: workflow\ndescription: Project workflow.\ncustom: preserve\n---\n# Workflow\n"
	input := map[string]any{"name": "workflow", "content": source, "scopeKey": catalog.ScopeKey}
	var created skills.Detail
	f.call("POST", route, input, 201, &created)
	if created.Source != source || created.Hash == "" || !created.Editable || !created.Deletable {
		t.Fatal("creation did not expose source and management capabilities")
	}
	f.call("POST", route, input, 409, nil)
	f.call("DELETE", route+"/"+created.ID, map[string]any{"scopeKey": catalog.ScopeKey, "expectedRevision": created.Revision}, 400, nil)
	if _, err := os.Stat(filepath.Join(directory, ".agents/skills/workflow/SKILL.md")); err != nil {
		t.Fatal("unconfirmed deletion removed a skill")
	}
	f.call("PUT", route+"/"+created.ID, map[string]any{"content": source + "Updated.\n", "expectedHash": created.Hash, "scopeKey": catalog.ScopeKey}, 200, &created)
	updatedHash := created.Hash
	f.stop()
	f.start()
	f.waitReady()
	f.call("GET", route+"/"+created.ID, nil, 200, &created)
	if created.Source != source+"Updated.\n" || created.Hash != updatedHash {
		t.Fatal("restart lost full source or extra front matter")
	}
	asset := filepath.Join(directory, ".agents/skills/workflow/example.txt")
	if err := os.WriteFile(asset, []byte("Supporting data"), 0644); err != nil {
		t.Fatal(err)
	}
	f.call("GET", route, nil, 200, &catalog)
	f.call("DELETE", route+"/"+created.ID, map[string]any{"scopeKey": catalog.ScopeKey, "expectedRevision": catalog.Skills[0].Revision, "confirmed": true}, 200, nil)
	f.call("GET", route, nil, 200, &catalog)
	if len(catalog.Skills) != 0 {
		t.Fatal("deleted skill remained in catalog")
	}
	if _, err := os.Stat(asset); !os.IsNotExist(err) {
		t.Fatal("owned supporting file remained")
	}
	f.call("GET", route+"/"+created.ID, nil, 404, nil)
	f.native.mu.Lock()
	defer f.native.mu.Unlock()
	if f.native.starts != 0 {
		t.Fatal("skill management submitted model work")
	}
}

func TestSkillManagementRejectsStaleEditsAndSerializesDirectoryWriters(t *testing.T) {
	f, id, directory := skillManagementFixture(t)
	route := "/projects/" + id + "/skills"
	var catalog controller.ProjectSkillCatalog
	f.call("GET", route, nil, 200, &catalog)
	var created skills.Detail
	f.call("POST", route, map[string]any{"name": "workflow", "content": "Original source", "scopeKey": catalog.ScopeKey}, 201, &created)
	file := filepath.Join(directory, ".agents/skills/workflow/SKILL.md")
	if err := os.WriteFile(file, []byte("External source"), 0644); err != nil {
		t.Fatal(err)
	}
	f.call("PUT", route+"/"+created.ID, map[string]any{"content": "Stale edit", "expectedHash": created.Hash, "scopeKey": catalog.ScopeKey}, 409, nil)
	f.call("DELETE", route+"/"+created.ID, map[string]any{"expectedRevision": created.Revision, "scopeKey": catalog.ScopeKey, "confirmed": true}, 409, nil)
	if raw, _ := os.ReadFile(file); string(raw) != "External source" {
		t.Fatal("stale request replaced external edits")
	}
	for _, input := range []map[string]any{
		{"name": "invalid\nname", "content": "Body", "scopeKey": catalog.ScopeKey},
		{"name": "large", "content": strings.Repeat("x", (256<<10)+1), "scopeKey": catalog.ScopeKey},
		{"name": "invalid", "content": "---\nname: [\n---\nBody", "scopeKey": catalog.ScopeKey},
	} {
		f.call("POST", route, input, 400, nil)
	}
	f.call("POST", route, map[string]any{"name": "stale", "content": "Body", "scopeKey": "stale"}, 409, nil)
	var session struct{ ID string }
	f.call("POST", "/sessions", map[string]any{"endpointId": f.endpoint, "cwd": directory}, 201, &session)
	task := f.submit(session.ID, "active-skill-task", "Use this project")
	f.call("POST", route, map[string]any{"name": "blocked", "content": "Body", "scopeKey": catalog.ScopeKey}, 409, nil)
	f.call("GET", route, nil, 200, &catalog)
	f.call("DELETE", route+"/"+created.ID, map[string]any{"expectedRevision": catalog.Skills[0].Revision, "scopeKey": catalog.ScopeKey, "confirmed": true}, 409, nil)
	f.native.finish(task.Session.ProviderID, task.Turns[0].ProviderID, "completed", "Done")
	eventually(t, func() bool { return f.snapshot(session.ID).Session.State == "idle" })
	f.call("POST", route, map[string]any{"name": "available", "content": "Body", "scopeKey": catalog.ScopeKey}, 201, nil)
}

func TestSkillCreationIsExclusiveAcrossConcurrentRequests(t *testing.T) {
	f, id, _ := skillManagementFixture(t)
	route := "/projects/" + id + "/skills"
	var catalog controller.ProjectSkillCatalog
	f.call("GET", route, nil, 200, &catalog)
	var wg sync.WaitGroup
	statuses := make(chan int, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			status, _, err := f.request("POST", route, map[string]any{"name": "one", "content": "Complete source", "scopeKey": catalog.ScopeKey})
			if err != nil {
				statuses <- 0
			} else {
				statuses <- status
			}
		}()
	}
	wg.Wait()
	close(statuses)
	counts := map[int]int{}
	for status := range statuses {
		counts[status]++
	}
	if counts[201] != 1 || counts[409] != 1 {
		t.Fatalf("creation statuses: %+v", counts)
	}
}

func TestSkillMutationsRemainInProjectSessionAndBranchScope(t *testing.T) {
	f, id, directory := skillManagementFixture(t)
	gitCommand(t, directory, "init", "-b", "main")
	route := "/projects/" + id + "/skills"
	var catalog controller.ProjectSkillCatalog
	f.call("GET", route, nil, 200, &catalog)
	f.call("POST", route, map[string]any{"name": "unborn", "content": "Before the first commit", "scopeKey": catalog.ScopeKey}, 201, nil)
	gitCommand(t, directory, "config", "user.name", "Loom Test")
	gitCommand(t, directory, "config", "user.email", "loom@example.test")
	gitCommand(t, directory, "add", ".")
	gitCommand(t, directory, "commit", "-m", "Initial")
	f.call("GET", route, nil, 200, &catalog)
	gitCommand(t, directory, "switch", "-c", "feature/revised")
	f.call("POST", route, map[string]any{"name": "stale", "content": "Body", "scopeKey": catalog.ScopeKey}, 409, nil)
	var otherProject, otherSession struct{ ID string }
	f.call("POST", "/projects", map[string]any{"endpointId": f.endpoint, "cwd": t.TempDir()}, 201, &otherProject)
	f.call("POST", "/sessions", map[string]any{"endpointId": f.endpoint, "cwd": t.TempDir()}, 201, &otherSession)
	f.call("POST", route+"?sessionId="+otherSession.ID, map[string]any{"name": "wrong", "content": "Body", "scopeKey": catalog.ScopeKey}, 400, nil)
	f.call("POST", "/projects/missing/skills", map[string]any{"name": "wrong", "content": "Body", "scopeKey": catalog.ScopeKey}, 404, nil)
}

func TestSkillDisplayNamesPersistAcrossRenamingAndRestart(t *testing.T) {
	f, id, directory := skillManagementFixture(t)
	route := "/projects/" + id + "/skills"
	var catalog controller.ProjectSkillCatalog
	f.call("GET", route, nil, 200, &catalog)
	name := "\u9700\u6c42\u5bf9\u9f50"
	var created skills.Detail
	f.call("POST", route, map[string]any{"name": name, "content": "# Instructions\n\nAlign requirement documents.", "scopeKey": catalog.ScopeKey}, 201, &created)
	if created.Name != name || !strings.HasPrefix(created.Paths[0], ".agents/skills/skill-") {
		t.Fatal("Unicode display name was used as a path")
	}
	path := created.Paths[0]
	asset := filepath.Join(directory, filepath.Dir(filepath.FromSlash(path)), "template.txt")
	if err := os.WriteFile(asset, []byte("Keep supporting file"), 0644); err != nil {
		t.Fatal(err)
	}
	renamed := "\u8bbe\u8ba1\u5bf9\u9f50"
	f.call("PUT", route+"/"+created.ID, map[string]any{"name": renamed, "content": created.Source, "expectedHash": created.Hash, "scopeKey": catalog.ScopeKey}, 200, &created)
	if created.Name != renamed || created.Paths[0] != path {
		t.Fatal("renaming moved the installation")
	}
	f.stop()
	f.start()
	f.waitReady()
	f.call("GET", route+"/"+created.ID, nil, 200, &created)
	if created.Name != renamed || created.Paths[0] != path {
		t.Fatal("restart lost the display name")
	}
	if raw, err := os.ReadFile(asset); err != nil || string(raw) != "Keep supporting file" {
		t.Fatal("renaming lost supporting files")
	}
	f.call("GET", route, nil, 200, &catalog)
	if len(catalog.Skills) != 1 || catalog.Skills[0].Name != renamed {
		t.Fatal("catalog did not expose the display name")
	}
	f.call("DELETE", route+"/"+created.ID, map[string]any{"scopeKey": catalog.ScopeKey, "expectedRevision": catalog.Skills[0].Revision, "confirmed": true}, 200, nil)
	if _, err := os.Stat(asset); !os.IsNotExist(err) {
		t.Fatal("confirmation did not remove the named installation")
	}
	for _, invalid := range []string{"", "line\nbreak", strings.Repeat("x", 129)} {
		f.call("POST", route, map[string]any{"name": invalid, "content": "Instructions", "scopeKey": catalog.ScopeKey}, 400, nil)
	}
}
