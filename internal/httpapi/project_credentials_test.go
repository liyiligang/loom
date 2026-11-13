package httpapi_test

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func credentialProjects(t *testing.T) (*fixture, string, string, string, string) {
	f, _ := localFixture(t, true, nil)
	f.localReady()
	f.call("POST", "/local/confirm", nil, 200, nil)
	f.endpoint = "local-codex"
	f.waitReady()
	var first, second, one, two struct{ ID string }
	f.call("POST", "/projects", map[string]any{"endpointId": f.endpoint, "cwd": t.TempDir(), "name": "First project"}, 201, &first)
	f.call("POST", "/projects", map[string]any{"endpointId": f.endpoint, "cwd": t.TempDir(), "name": "Second project"}, 201, &second)
	var navigation navigation
	f.call("GET", "/bootstrap", nil, 200, &navigation)
	for _, requirement := range navigation.Requirements {
		if requirement.ProjectID == first.ID {
			f.call("POST", "/sessions", map[string]any{"endpointId": f.endpoint, "requirementId": requirement.ID}, 201, &one)
		}
		if requirement.ProjectID == second.ID {
			f.call("POST", "/sessions", map[string]any{"endpointId": f.endpoint, "requirementId": requirement.ID}, 201, &two)
		}
	}
	return f, first.ID, second.ID, one.ID, two.ID
}

func TestProjectCredentialGrantsEnforceScopeAndCancelRevokedUse(t *testing.T) {
	f, first, second, one, two := credentialProjects(t)
	var token, keys struct{ ID string }
	f.call("POST", "/secrets", map[string]any{"name": "Platform token", "kind": "token", "token": "dummy-scoped-token"}, 201, &token)
	f.call("POST", "/secrets", map[string]any{"name": "Cloud keys", "kind": "accessKey", "accessKeyId": "dummy-ak", "secretAccessKey": "dummy-sk"}, 201, &keys)
	firstTurn := f.submit(one, "scope-one", "Use activated credentials")
	secondTurn := f.submit(two, "scope-two", "Keep project credentials isolated")
	if records, err := f.service.ListSessionCredentials(one); err != nil || len(records) != 0 {
		t.Fatal("project inherited credentials without activation")
	}
	f.call("PATCH", "/projects/"+first, map[string]any{"secretIds": []string{token.ID, keys.ID}}, 200, nil)
	if records, err := f.service.ListSessionCredentials(one); err != nil || len(records) != 2 {
		t.Fatal("activated credentials were unavailable")
	}
	if _, err := f.service.LeaseSessionCredential(context.Background(), two, token.ID); err == nil {
		t.Fatal("another project could use an inactive credential")
	}
	lease, err := f.service.LeaseSessionCredential(context.Background(), one, token.ID)
	if err != nil || lease.Values.Token != "dummy-scoped-token" {
		t.Fatal("selected credential was not resolved")
	}
	f.call("PATCH", "/projects/"+first, map[string]any{"secretIds": []string{keys.ID}}, 200, nil)
	if lease.Context.Err() == nil {
		t.Fatal("deactivation did not cancel the active use")
	}
	lease.Release()
	if _, err := f.service.LeaseSessionCredential(context.Background(), one, token.ID); err == nil {
		t.Fatal("deactivated credential remained usable")
	}
	f.call("PATCH", "/projects/"+first, map[string]any{"secretIds": []string{token.ID, keys.ID}}, 200, nil)
	lease, err = f.service.LeaseSessionCredential(context.Background(), one, token.ID)
	if err != nil {
		t.Fatal(err)
	}
	f.call("PATCH", "/secrets/"+token.ID, map[string]any{"token": "dummy-rotated-token"}, 200, nil)
	if lease.Context.Err() == nil {
		t.Fatal("rotation did not cancel old credential use")
	}
	lease.Release()
	f.call("PATCH", "/projects/"+second, map[string]any{"secretIds": []string{token.ID, keys.ID}}, 200, nil)
	f.call("DELETE", "/secrets/"+token.ID, nil, 200, nil)
	var data navigation
	f.call("GET", "/bootstrap", nil, 200, &data)
	for _, project := range data.Projects {
		if slices.Contains(project.SecretIDs, token.ID) || !slices.Contains(project.SecretIDs, keys.ID) {
			t.Fatal("deletion did not update all project grants atomically")
		}
	}
	raw, _ := json.Marshal(data)
	if strings.Contains(string(raw), "dummy-") {
		t.Fatal("project metadata contained a credential value")
	}
	f.native.finish(firstTurn.Session.ProviderID, firstTurn.Turns[0].ProviderID, "completed", "Done")
	f.native.finish(secondTurn.Session.ProviderID, secondTurn.Turns[0].ProviderID, "completed", "Done")
	eventually(t, func() bool { return f.snapshot(one).Session.State == "idle" && f.snapshot(two).Session.State == "idle" })
	f.stop()
	f.start()
	f.waitReady()
	eventually(t, func() bool { return f.snapshot(one).Session.State == "idle" && f.snapshot(two).Session.State == "idle" })
	f.call("GET", "/bootstrap", nil, 200, &data)
	for _, project := range data.Projects {
		if len(project.SecretIDs) != 1 || project.SecretIDs[0] != keys.ID {
			t.Fatal("restart lost project activation")
		}
	}
	if _, err := f.service.LeaseSessionCredential(context.Background(), one, keys.ID); err == nil {
		t.Fatal("idle session could execute credentialed requests")
	}
}

func TestProjectCredentialValidationDoesNotPartiallyRenameOrActivate(t *testing.T) {
	f, first, _, _, _ := credentialProjects(t)
	var saved struct{ ID string }
	f.call("POST", "/secrets", map[string]any{"name": "Platform", "kind": "token", "token": "dummy-value"}, 201, &saved)
	f.call("PATCH", "/projects/"+first, map[string]any{"name": "Must not change", "secretIds": []string{"missing"}}, 404, nil)
	f.call("PATCH", "/projects/"+first, map[string]any{"secretIds": []string{saved.ID, saved.ID}}, 400, nil)
	f.call("PATCH", "/projects/"+first, map[string]any{}, 400, nil)
	var data navigation
	f.call("GET", "/bootstrap", nil, 200, &data)
	if data.Projects[0].Name != "First project" || len(data.Projects[0].SecretIDs) != 0 {
		t.Fatal("invalid activation changed project metadata")
	}
	f.call("PATCH", "/projects/"+first, map[string]any{"secretIds": []string{saved.ID}}, 200, nil)
	f.call("PATCH", "/projects/"+first, map[string]any{"name": "Renamed project"}, 200, nil)
	f.call("GET", "/bootstrap", nil, 200, &data)
	if data.Projects[0].Name != "Renamed project" || !slices.Contains(data.Projects[0].SecretIDs, saved.ID) {
		t.Fatal("renaming discarded project activation")
	}
	f.call("PATCH", "/projects/"+first, map[string]any{"secretIds": []string{}}, 200, nil)
	data = navigation{}
	f.call("GET", "/bootstrap", nil, 200, &data)
	if len(data.Projects[0].SecretIDs) != 0 {
		t.Fatal("explicit empty list did not disable credentials")
	}
}

func TestMovingAnActiveSessionRechecksItsNewProjectCredentialScope(t *testing.T) {
	f, first, second, one, _ := credentialProjects(t)
	var saved struct{ ID string }
	f.call("POST", "/secrets", map[string]any{"name": "Platform", "kind": "token", "token": "dummy-value"}, 201, &saved)
	f.call("PATCH", "/projects/"+first, map[string]any{"secretIds": []string{saved.ID}}, 200, nil)
	f.submit(one, "scope-move", "Use project credentials")
	lease, err := f.service.LeaseSessionCredential(context.Background(), one, saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	var data navigation
	f.call("GET", "/bootstrap", nil, 200, &data)
	for _, requirement := range data.Requirements {
		if requirement.ProjectID == second {
			f.call("PATCH", "/sessions/"+one, map[string]any{"requirementId": requirement.ID}, 200, nil)
			break
		}
	}
	if lease.Context.Err() == nil {
		t.Fatal("moving the session did not cancel its old grant")
	}
	lease.Release()
	if _, err := f.service.LeaseSessionCredential(context.Background(), one, saved.ID); err == nil {
		t.Fatal("session used its previous project's grant")
	}
}
