package httpapi_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/liyiligang/loom/internal/materials"
)

func TestPrototypePreviewIsVersionedIsolatedAndFileBacked(t *testing.T) {
	f, id, directory := materialFixture(t)
	route := "/requirements/" + id + "/materials"
	var catalog materials.Catalog
	f.call("GET", route, nil, 200, &catalog)
	body := "<!doctype html><title>Prototype</title><button id=action>Try demo</button><script>document.querySelector('#action').onclick = () => document.title = 'Clicked'</script>"
	var prototype, design materials.Detail
	f.call("POST", route, map[string]any{"kind": "prototypes", "name": "demo.html", "content": body, "scopeKey": catalog.Scope.Key}, 201, &prototype)
	f.call("POST", route, map[string]any{"kind": "designDocuments", "name": "design.md", "content": "# Demo design\n", "scopeKey": catalog.Scope.Key}, 201, &design)
	f.call("PUT", route+"/"+prototype.ID+"/links", map[string]any{"scopeKey": catalog.Scope.Key, "relatedIds": []string{design.ID}}, 200, nil)
	query := url.Values{"scopeKey": {catalog.Scope.Key}, "hash": {prototype.Hash}}
	preview := route + "/" + prototype.ID + "/preview?" + query.Encode()
	response, err := http.Get(f.server.URL + "/api/v1" + preview)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || response.StatusCode != 200 || string(raw) != body || response.Header.Get("Content-Type") != "text/html; charset=utf-8" {
		t.Fatal("preview did not return the exact saved HTML")
	}
	csp := response.Header.Get("Content-Security-Policy")
	for _, directive := range []string{"sandbox allow-scripts allow-forms", "script-src 'unsafe-inline'", "connect-src 'none'", "default-src 'none'", "form-action 'none'", "frame-ancestors 'self'"} {
		if !strings.Contains(csp, directive) {
			t.Fatalf("missing preview boundary: %s", directive)
		}
	}
	if strings.Contains(csp, "allow-same-origin") || response.Header.Get("X-Frame-Options") != "SAMEORIGIN" || response.Header.Get("Cache-Control") != "no-store" || response.Header.Get("Referrer-Policy") != "no-referrer" {
		t.Fatal("preview gained application origin access or caching")
	}
	application, err := http.Get(f.server.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	application.Body.Close()
	if application.Header.Get("X-Frame-Options") != "DENY" || !strings.Contains(application.Header.Get("Content-Security-Policy"), "script-src 'self'") {
		t.Fatal("prototype embedding weakened the main application policy")
	}
	f.call("GET", route+"/"+prototype.ID+"/preview", nil, 409, nil)
	f.call("GET", route+"/"+design.ID+"/preview?"+query.Encode(), nil, 400, nil)
	f.stop()
	f.start()
	f.waitReady()
	f.call("GET", route+"/"+prototype.ID, nil, 200, &prototype)
	if prototype.Content != body || len(prototype.RelatedIDs) != 1 || prototype.RelatedIDs[0] != design.ID {
		t.Fatal("restart lost prototype content or design relationship")
	}
	if err := os.WriteFile(filepath.Join(directory, prototype.Path), []byte("<!doctype html><title>Updated demo</title>"), 0644); err != nil {
		t.Fatal(err)
	}
	f.call("GET", preview, nil, 409, nil)
	f.call("GET", route+"/"+prototype.ID, nil, 200, &prototype)
	query.Set("hash", prototype.Hash)
	f.call("GET", route+"/"+prototype.ID+"/preview?"+query.Encode(), nil, 200, nil)
	large := filepath.Join(directory, catalog.Scope.Root, "prototypes", "large.html")
	if err := os.WriteFile(large, []byte(strings.Repeat("x", materials.MaxEditBytes+1)), 0644); err != nil {
		t.Fatal(err)
	}
	f.call("GET", route, nil, 200, &catalog)
	for _, file := range catalog.Files {
		if filepath.Base(file.Path) == "large.html" {
			query.Set("hash", file.Hash)
			f.call("GET", route+"/"+file.ID+"/preview?"+query.Encode(), nil, 400, nil)
		}
	}
	var discussion struct{ SessionID, TurnID string }
	f.call("POST", "/requirements/"+id+"/conversation/turns", map[string]any{"text": "Refine this prototype", "clientRequestId": "prototype-context", "section": "prototypes"}, 202, &discussion)
	eventually(t, func() bool { return f.snapshot(discussion.SessionID).Turns[0].ProviderID != "" })
	turn := f.snapshot(discussion.SessionID).Turns[0]
	if !strings.Contains(turn.Context, prototype.Path) || !strings.Contains(turn.Context, "self-contained .html") || !strings.Contains(turn.Context, `"section":"Prototypes"`) {
		t.Fatal("AI did not receive prototype files and generation rules")
	}
}

func materialFixture(t *testing.T) (*fixture, string, string) {
	t.Helper()
	f, _ := localFixture(t, true, nil)
	_ = f.localReady()
	f.call("POST", "/local/confirm", nil, 200, nil)
	f.waitReady()
	f.endpoint = "local-codex"
	directory := gitProject(t)
	var project, requirement struct{ ID string }
	f.call("POST", "/projects", map[string]any{"endpointId": f.endpoint, "cwd": directory}, 201, &project)
	f.call("POST", "/requirements", map[string]any{"projectId": project.ID, "name": "File planning"}, 201, &requirement)
	return f, requirement.ID, directory
}

func TestRequirementMaterialsPersistFilesAndRelationshipsAcrossRestart(t *testing.T) {
	f, id, directory := materialFixture(t)
	route := "/requirements/" + id + "/materials"
	var catalog materials.Catalog
	f.call("GET", route, nil, 200, &catalog)
	if len(catalog.Files) != 0 || catalog.Scope.Root != "docs/requirements/"+id || catalog.Scope.Branch != "main" {
		t.Fatal("unexpected empty project scope")
	}
	var doc, test materials.Detail
	input := map[string]any{"kind": "documents", "name": "requirements.md", "content": "# Sign in\n\nUse account credentials.\n", "scopeKey": catalog.Scope.Key}
	f.call("POST", route, input, 201, &doc)
	f.call("POST", route, map[string]any{"kind": "testCases", "name": "cases.yaml", "content": "cases:\n  - name: Valid credentials\n", "scopeKey": catalog.Scope.Key}, 201, &test)
	f.call("PUT", route+"/"+doc.ID+"/links", map[string]any{"scopeKey": catalog.Scope.Key, "relatedIds": []string{test.ID}}, 200, nil)
	f.call("PUT", route+"/"+doc.ID+"/links", map[string]any{"scopeKey": catalog.Scope.Key, "relatedIds": []string{"missing"}}, 404, nil)
	f.call("PUT", route+"/"+doc.ID+"/links", map[string]any{"scopeKey": catalog.Scope.Key, "relatedIds": []string{doc.ID}}, 400, nil)
	raw, err := os.ReadFile(filepath.Join(directory, doc.Path))
	if err != nil || string(raw) != input["content"] {
		t.Fatal("document body was not persisted in the project")
	}
	status, download, err := f.request("GET", route+"/"+doc.ID+"/download", nil)
	if err != nil || status != 200 || string(download) != string(raw) {
		t.Fatal("download did not serve the stored file")
	}
	f.stop()
	f.start()
	f.waitReady()
	f.call("GET", route+"/"+doc.ID, nil, 200, &doc)
	if doc.Content != string(raw) || len(doc.RelatedIDs) != 1 || doc.RelatedIDs[0] != test.ID {
		t.Fatal("restart lost file content or indexed relationships")
	}
	if err := os.WriteFile(filepath.Join(directory, doc.Path), []byte("# External update\n"), 0644); err != nil {
		t.Fatal(err)
	}
	input["expectedHash"] = doc.Hash
	input["content"] = "# Stale browser update\n"
	f.call("PUT", route+"/"+doc.ID, input, 409, nil)
	f.call("GET", route+"/"+doc.ID, nil, 200, &doc)
	if doc.Content != "# External update\n" || len(doc.RelatedIDs) != 1 {
		t.Fatal("refresh overwrote an external edit or discarded links")
	}
	input["expectedHash"] = doc.Hash
	input["content"] = "# Browser update\n"
	f.call("PUT", route+"/"+doc.ID, input, 200, &doc)
	input["expectedHash"] = ""
	f.call("POST", route, input, 409, nil)
	input["name"] = "../escape.md"
	f.call("POST", route, input, 400, nil)
	input["name"] = "large.md"
	input["content"] = strings.Repeat("x", materials.MaxEditBytes+1)
	f.call("POST", route, input, 400, nil)
	input["content"] = "Text"
	input["scopeKey"] = "stale-branch"
	f.call("POST", route, input, 409, nil)
	f.call("GET", route+"/missing", nil, 404, nil)
	if err := os.Remove(filepath.Join(directory, test.Path)); err != nil {
		t.Fatal(err)
	}
	f.call("GET", route+"/"+doc.ID, nil, 200, &doc)
	if len(doc.RelatedIDs) != 0 {
		t.Fatal("removed file retained dangling relationships")
	}
	f.native.mu.Lock()
	defer f.native.mu.Unlock()
	if f.native.starts != 0 {
		t.Fatal("file management started an AI task")
	}
}

func TestMaterialsFollowWorktreeAndSerializeWithRequirementAI(t *testing.T) {
	f, id, directory := materialFixture(t)
	route := "/requirements/" + id + "/materials"
	f.call("PATCH", "/requirements/"+id, map[string]any{"branch": "feature/materials-first"}, 200, nil)
	first := f.waitWorkspace(id, "ready")
	var catalog materials.Catalog
	f.call("GET", route, nil, 200, &catalog)
	firstScope := catalog.Scope
	if firstScope.Directory != first.Workspace.Directory || firstScope.Branch != first.Workspace.Branch {
		t.Fatal("materials did not select the bound worktree")
	}
	var doc materials.Detail
	input := map[string]any{"kind": "designDocuments", "name": "design.md", "content": "# First design\n", "scopeKey": firstScope.Key}
	f.call("POST", route, input, 201, &doc)
	var discussion struct{ SessionID, TurnID string }
	f.call("POST", "/requirements/"+id+"/conversation/turns", map[string]any{"text": "Review this design", "clientRequestId": "materials-first", "section": "designDocuments"}, 202, &discussion)
	eventually(t, func() bool { return f.snapshot(discussion.SessionID).Turns[0].ProviderID != "" })
	before := f.snapshot(discussion.SessionID)
	if before.Session.MaterialDirectory != firstScope.Directory || before.Session.Cwd == firstScope.Directory || !strings.Contains(before.Turns[0].Context, doc.Path) || !strings.Contains(before.Turns[0].Context, doc.Hash) {
		t.Fatal("discussion did not receive the current file catalog and material directory")
	}
	var feature struct{ ID string }
	f.call("POST", "/sessions", map[string]any{"endpointId": f.endpoint, "requirementId": id}, 201, &feature)
	f.call("POST", "/sessions/"+feature.ID+"/turns", map[string]any{"text": "Conflicting edit", "clientRequestId": "blocked-feature"}, 409, nil)
	input["expectedHash"] = doc.Hash
	f.call("PUT", route+"/"+doc.ID, input, 409, nil)
	f.call("GET", route+"/"+doc.ID, nil, 200, &doc)
	f.call("PATCH", "/requirements/"+id, map[string]any{"branch": "feature/materials-second"}, 200, nil)
	second := f.waitWorkspace(id, "ready")
	f.call("GET", route, nil, 200, &catalog)
	if catalog.Scope.Key == firstScope.Key || catalog.Scope.Directory != second.Workspace.Directory || len(catalog.Files) != 0 {
		t.Fatal("branch replacement reused the previous file scope")
	}
	delete(input, "expectedHash")
	f.call("POST", route, input, 409, nil)
	input["scopeKey"] = catalog.Scope.Key
	input["content"] = "# Second design\n"
	f.call("POST", route, input, 201, &doc)
	if f.snapshot(discussion.SessionID).Session.MaterialDirectory != firstScope.Directory {
		t.Fatal("branch selection repointed an active AI task")
	}
	f.native.finish(before.Session.ProviderID, before.Turns[0].ProviderID, "completed", "Reviewed first design")
	eventually(t, func() bool { return f.snapshot(discussion.SessionID).Session.State == "idle" })
	var next struct{ SessionID, TurnID string }
	f.call("POST", "/requirements/"+id+"/conversation/turns", map[string]any{"text": "Review the revised branch", "clientRequestId": "materials-second", "section": "designDocuments"}, 202, &next)
	eventually(t, func() bool { return f.snapshot(discussion.SessionID).Turns[1].ProviderID != "" })
	after := f.snapshot(discussion.SessionID)
	encodedDirectory, err := json.Marshal(second.Workspace.Directory)
	if err != nil {
		t.Fatal(err)
	}
	if next.SessionID != discussion.SessionID || after.Session.MaterialDirectory != second.Workspace.Directory || !strings.Contains(after.Turns[1].Context, string(encodedDirectory)) || !strings.Contains(after.Turns[1].Context, `"branch":"feature/materials-second"`) {
		t.Fatal("next discussion turn did not select the revised branch")
	}
	f.native.finish(after.Session.ProviderID, after.Turns[1].ProviderID, "completed", "Reviewed second design")
	eventually(t, func() bool { return f.snapshot(discussion.SessionID).Session.State == "idle" })
	f.stop()
	f.start()
	f.waitReady()
	eventually(t, func() bool { return f.snapshot(discussion.SessionID).Session.State == "idle" })
	if f.snapshot(discussion.SessionID).Session.MaterialDirectory != second.Workspace.Directory {
		t.Fatal("restart lost the last AI material directory")
	}
	f.call("PATCH", "/requirements/"+id, map[string]any{"branch": first.Workspace.Branch}, 200, nil)
	f.waitWorkspace(id, "ready")
	f.call("GET", route+"/"+doc.ID, nil, 200, &doc)
	if doc.Content != "# First design\n" {
		t.Fatal("switching back lost the first branch's material")
	}
	if _, err := os.Stat(filepath.Join(directory, doc.Path)); !os.IsNotExist(err) {
		t.Fatal("bound material was written into the source checkout")
	}
}

func TestMaterialsRejectRemoteProjectAccess(t *testing.T) {
	f := setup(t)
	id := requirementForChat(t, f)
	f.call("GET", "/requirements/"+id+"/materials", nil, 409, nil)
	f.call("GET", "/requirements/missing/materials", nil, 404, nil)
}
