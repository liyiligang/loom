package httpapi_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/liyiligang/loom/internal/controller"
	"github.com/liyiligang/loom/internal/model"
	"github.com/liyiligang/loom/internal/skills"
	"github.com/liyiligang/loom/internal/storage"
)

func installSkill(t *testing.T, directory, name, body string) string {
	t.Helper()
	path := filepath.Join(directory, ".agents/skills", name, "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("---\nname: "+name+"\ndescription: Project guidance.\n---\n"+body), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestProjectSkillsRequireConsentAndRejectRemotePaths(t *testing.T) {
	directory, dataPath := t.TempDir(), t.TempDir()
	installSkill(t, directory, "example", "Local text")
	store, err := storage.Open(filepath.Join(dataPath, "loom.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Commit("navigation.updated", "", &model.Project{ID: "local", Local: true, Cwd: directory}, &model.Project{ID: "remote", Cwd: directory}); err != nil {
		t.Fatal(err)
	}
	store.Close()
	f := &fixture{t: t, dataPath: dataPath, manager: &localManager{}}
	f.start()
	t.Cleanup(f.stop)
	for _, path := range []string{"/projects/local/skills", "/projects/local/skills/example"} {
		var response struct{ Error struct{ Code string } }
		f.call("GET", path, nil, 409, &response)
		if response.Error.Code != "local_unconfirmed" {
			t.Fatalf("local consent bypass: %+v", response)
		}
	}
	var response struct{ Error struct{ Code string } }
	f.call("GET", "/projects/remote/skills", nil, 409, &response)
	if response.Error.Code != "remote_skills" {
		t.Fatalf("remote path read locally: %+v", response)
	}
	f.call("GET", "/projects/missing/skills", nil, 404, nil)
	f.call("GET", "/projects/local/git", nil, 409, nil)
	f.call("POST", "/requirements", map[string]any{"projectId": "local", "name": "Unconfirmed", "branch": "feature/no-consent"}, 409, nil)
}

func TestProjectSkillsUseStoredProjectAndSessionDirectories(t *testing.T) {
	f, _ := localFixture(t, true, nil)
	_ = f.localReady()
	f.call("POST", "/local/confirm", nil, 200, nil)
	f.waitReady()
	directory, worktree, other := t.TempDir(), t.TempDir(), t.TempDir()
	installSkill(t, directory, "root-skill", "Root text")
	worktreeFile := installSkill(t, worktree, "feature-skill", "Feature text")
	installSkill(t, other, "other-skill", "Other text")
	var project, otherProject, session, otherSession struct{ ID string }
	f.call("POST", "/projects", map[string]any{"endpointId": "local-codex", "cwd": directory}, 201, &project)
	f.call("POST", "/projects", map[string]any{"endpointId": "local-codex", "cwd": other}, 201, &otherProject)
	var navigation navigation
	f.call("GET", "/bootstrap", nil, 200, &navigation)
	var requirement, otherRequirement string
	for _, value := range navigation.Requirements {
		if value.ProjectID == project.ID {
			requirement = value.ID
		}
		if value.ProjectID == otherProject.ID {
			otherRequirement = value.ID
		}
	}
	f.call("POST", "/sessions", map[string]any{"endpointId": "local-codex", "requirementId": requirement, "cwd": worktree}, 201, &session)
	f.call("POST", "/sessions", map[string]any{"endpointId": "local-codex", "requirementId": otherRequirement}, 201, &otherSession)
	base := "/projects/" + project.ID + "/skills"
	var root, feature controller.ProjectSkillCatalog
	f.call("GET", base+"?path="+other, nil, 200, &root)
	canonical, _ := filepath.EvalSymlinks(directory)
	if root.Directory != canonical || root.ProjectID != project.ID || len(root.Skills) != 1 || root.Skills[0].Name != "root-skill" {
		t.Fatalf("caller path replaced registered directory: %+v", root)
	}
	f.call("GET", base+"?sessionId="+session.ID, nil, 200, &feature)
	if len(feature.Skills) != 1 || feature.Skills[0].Name != "feature-skill" {
		t.Fatalf("session did not use its worktree: %+v", feature)
	}
	var preview skills.Detail
	f.call("GET", base+"/"+feature.Skills[0].ID+"?sessionId="+session.ID, nil, 200, &preview)
	if preview.Content != "Feature text" {
		t.Fatalf("wrong skill content: %+v", preview)
	}
	f.call("GET", base+"/"+feature.Skills[0].ID, nil, 404, nil)
	f.call("GET", base+"?sessionId="+otherSession.ID, nil, 400, nil)
	f.call("GET", base+"?sessionId=missing", nil, 404, nil)
	f.call("GET", base+"/"+root.Skills[0].ID+"?sessionId="+otherSession.ID, nil, 400, nil)
	if err := os.WriteFile(worktreeFile, []byte(strings.Repeat("a", 300<<10)), 0644); err != nil {
		t.Fatal(err)
	}
	f.call("GET", base+"/"+feature.Skills[0].ID+"?sessionId="+session.ID, nil, 400, nil)
	if err := os.RemoveAll(directory); err != nil {
		t.Fatal(err)
	}
	f.call("GET", base, nil, 400, nil)
	f.native.mu.Lock()
	defer f.native.mu.Unlock()
	if f.native.starts != 0 {
		t.Fatal("skill browsing started a model turn")
	}
}
