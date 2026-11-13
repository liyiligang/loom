package storage

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/liyiligang/loom/internal/model"
	"github.com/liyiligang/loom/internal/workspace"
)

func TestSchemaFiveMigratesCredentialStorageAtomically(t *testing.T) {
	path := filepath.Join(t.TempDir(), "loom.db")
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, migration := range []string{initialMigration, localMigration, navigationMigration, workspaceMigration, workspaceHistoryMigration} {
		if _, err := legacy.Exec(migration); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := legacy.Exec(`INSERT INTO settings VALUES ('local_setup', '{"confirmed":true}')`); err != nil {
		t.Fatal(err)
	}
	legacy.Close()
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	first := &model.SecretRecord{Secret: model.Secret{ID: "one", Name: "Platform", Kind: "token"}, Ciphertext: []byte("ciphertext-only")}
	if _, err := store.Commit("secrets.updated", first.ID, first); err != nil {
		t.Fatal(err)
	}
	before, _ := store.Cursor()
	duplicate := &model.SecretRecord{Secret: model.Secret{ID: "two", Name: "platform", Kind: "token"}, Ciphertext: []byte("other-ciphertext")}
	if _, err := store.Commit("secrets.updated", duplicate.ID, duplicate); err == nil {
		t.Fatal("duplicate name was accepted")
	}
	after, _ := store.Cursor()
	data, err := store.Load()
	if err != nil || after != before || len(data.Secrets) != 1 || !data.LocalSetup.Confirmed {
		t.Fatal("migration or transaction rollback lost state")
	}
	raw, _ := json.Marshal(data.Secrets)
	if bytes.Contains(raw, first.Ciphertext) {
		t.Fatal("metadata exposed ciphertext")
	}
	if _, err := store.Commit("secrets.updated", first.ID, &model.SecretRemoval{ID: first.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SecretCiphertext(first.ID); err == nil {
		t.Fatal("deletion retained the record")
	}
}

func TestWorkspaceBindingsPersistAndBranchesHaveOneOwner(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "loom.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	project := &model.Project{ID: "p", Local: true, Cwd: "/repo"}
	first := &model.Requirement{ID: "r1", ProjectID: project.ID, Name: "First", Workspace: &workspace.Binding{CommonDir: "/repo/.git", Branch: "feature/one", BaseCommit: "pinned", Path: "/worktree/one", Directory: "/worktree/one", State: "preparing"}}
	if _, err := store.Commit("navigation.updated", first.ID, project, first); err != nil {
		t.Fatal(err)
	}
	second := &model.Requirement{ID: "r2", ProjectID: project.ID, Name: "Second", Workspace: &workspace.Binding{CommonDir: "/repo/.git", Branch: "feature/one"}}
	if _, err := store.Commit("navigation.updated", second.ID, second); err == nil {
		t.Fatal("duplicate repository branch was accepted")
	}
	loaded, err := store.Load()
	if err != nil || len(loaded.Requirements) != 1 || *loaded.Requirements[0].Workspace != *first.Workspace {
		t.Fatalf("intent lost or duplicate persisted: %+v, %v", loaded.Requirements, err)
	}
	second.Workspace.Branch = "feature/two"
	if _, err := store.Commit("navigation.updated", second.ID, second); err != nil {
		t.Fatal(err)
	}
	first.PreviousWorkspaces = []workspace.Binding{*first.Workspace}
	first.Workspace = &workspace.Binding{CommonDir: "/repo/.git", Branch: "feature/three"}
	if _, err := store.Commit("navigation.updated", first.ID, first); err != nil {
		t.Fatal(err)
	}
	second.Workspace.Branch = "feature/one"
	if _, err := store.Commit("navigation.updated", second.ID, second); err == nil {
		t.Fatal("retained branch was assigned to another requirement")
	}
	loaded, err = store.Load()
	if err != nil || loaded.Requirements[1].Workspace.Branch != "feature/two" || loaded.Requirements[0].PreviousWorkspaces[0].Branch != "feature/one" {
		t.Fatal("retained ownership or failed update rollback was lost")
	}
}

func TestWorkspaceHistoryMigrationReservesExistingBindings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "loom.db")
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, migration := range []string{initialMigration, localMigration, navigationMigration, workspaceMigration} {
		if _, err := legacy.Exec(migration); err != nil {
			t.Fatal(err)
		}
	}
	project := &model.Project{ID: "p", Local: true, Cwd: "/repo"}
	requirement := &model.Requirement{ID: "r", ProjectID: project.ID, Workspace: &workspace.Binding{CommonDir: "/repo/.git", Branch: "feature/existing"}}
	projectJSON, _ := json.Marshal(project)
	requirementJSON, _ := json.Marshal(requirement)
	if _, err := legacy.Exec("INSERT INTO projects VALUES (?, ?, ?, ?)", project.ID, "local", project.Cwd, projectJSON); err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.Exec("INSERT INTO requirements VALUES (?, ?, ?)", requirement.ID, project.ID, requirementJSON); err != nil {
		t.Fatal(err)
	}
	legacy.Close()
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var owner string
	if err := store.db.QueryRow("SELECT requirement_id FROM requirement_workspace_branches WHERE common_dir = ? AND branch = ?", requirement.Workspace.CommonDir, requirement.Workspace.Branch).Scan(&owner); err != nil || owner != requirement.ID {
		t.Fatal("schema 4 branch ownership did not migrate")
	}
}

func TestExistingDatabaseMigratesAndPersistsLocalConfirmation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "loom.db")
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.Exec(initialMigration); err != nil {
		t.Fatal(err)
	}
	legacy.Close()
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	data, err := store.Load()
	if err != nil || data.LocalSetup == nil || data.LocalSetup.Confirmed {
		t.Fatal("migration did not default to unconfirmed")
	}
	if _, err := store.Commit("local.updated", "", &model.LocalSetup{Confirmed: true}); err != nil {
		t.Fatal(err)
	}
	store.Close()
	store, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	data, err = store.Load()
	if err != nil || !data.LocalSetup.Confirmed {
		t.Fatal("confirmation did not survive restart")
	}
}

func TestNavigationMigrationPreservesLegacyRecords(t *testing.T) {
	for _, version := range []int{1, 2} {
		t.Run(fmt.Sprintf("schema%d", version), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "loom.db")
			legacy, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = legacy.Exec(initialMigration); err != nil {
				t.Fatal(err)
			}
			if version == 2 {
				if _, err = legacy.Exec(localMigration); err != nil {
					t.Fatal(err)
				}
				if _, err = legacy.Exec(`INSERT INTO settings VALUES ('local_setup', '{"confirmed":true}')`); err != nil {
					t.Fatal(err)
				}
			}
			values := []struct {
				query string
				args  []any
			}{
				{`INSERT INTO endpoints VALUES ('ep', ?)`, []any{`{"id":"ep","provider":"codex"}`}},
				{`INSERT INTO sessions VALUES ('s', 'ep', 'native', ?)`, []any{`{"id":"s","endpointId":"ep","providerId":"native","cwd":"/old/project","title":"Feature"}`}},
				{`INSERT INTO turns VALUES ('t', 's', 'request', ?)`, []any{`{"id":"t","sessionId":"s","clientRequestId":"request","state":"completed"}`}},
				{`INSERT INTO entries VALUES ('e', 's', 't', ?)`, []any{`{"id":"e","sessionId":"s","turnId":"t","text":"Saved answer"}`}},
			}
			for _, value := range values {
				if _, err = legacy.Exec(value.query, value.args...); err != nil {
					t.Fatal(err)
				}
			}
			legacy.Close()
			store, err := Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			data, err := store.Load()
			if err != nil {
				t.Fatal(err)
			}
			if data.LocalSetup.Confirmed != (version == 2) || len(data.Sessions) != 1 || data.Sessions[0].ProviderID != "native" || data.Entries[0].Text != "Saved answer" || data.Turns[0].ClientRequestID != "request" {
				t.Fatal("migration lost history or confirmation")
			}
			project := &model.Project{ID: "p", Name: "App", EndpointID: "ep", Cwd: "/old/project"}
			requirement := &model.Requirement{ID: "r", Name: "General", ProjectID: project.ID, DefaultName: true}
			data.Sessions[0].RequirementID = requirement.ID
			if _, err = store.Commit("navigation.updated", "s", project, requirement, data.Sessions[0]); err != nil {
				t.Fatal(err)
			}
			var raw string
			if err = store.db.QueryRow("SELECT requirement_id FROM sessions WHERE id='s'").Scan(&raw); err != nil || raw != "r" {
				t.Fatal("relational membership did not persist")
			}
			var schema int
			if err = store.db.QueryRow("SELECT max(version) FROM schema_migrations").Scan(&schema); err != nil || schema != 10 {
				t.Fatal("migration version incorrect")
			}
			data, err = store.Load()
			if err != nil || len(data.Projects) != 1 || len(data.Requirements) != 1 || data.Sessions[0].RequirementID != "r" {
				t.Fatal("group projections did not persist")
			}
			data.Sessions[0].RequirementID = "missing"
			if _, err = store.Commit("navigation.updated", "s", data.Sessions[0]); err == nil {
				t.Fatal("foreign key accepted a missing requirement")
			}
			var stored model.Session
			if err = store.db.QueryRow("SELECT data FROM sessions WHERE id='s'").Scan(&raw); err != nil {
				t.Fatal(err)
			}
			if err = json.Unmarshal([]byte(raw), &stored); err != nil || stored.RequirementID != "r" {
				t.Fatal("failed transaction changed session metadata")
			}
		})
	}
}

func TestSchemaSixProjectCredentialMigrationEnforcesReferences(t *testing.T) {
	path := filepath.Join(t.TempDir(), "loom.db")
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, migration := range []string{initialMigration, localMigration, navigationMigration, workspaceMigration, workspaceHistoryMigration, secretsMigration} {
		if _, err := legacy.Exec(migration); err != nil {
			t.Fatal(err)
		}
	}
	legacy.Close()
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	secret := &model.SecretRecord{Secret: model.Secret{ID: "secret", Name: "Platform", Kind: "token"}, Ciphertext: []byte("encrypted")}
	project := &model.Project{ID: "project", Name: "Project", Cwd: "/project", Local: true, SecretIDs: []string{secret.ID}}
	if _, err := store.Commit("navigation.updated", project.ID, secret, project); err != nil {
		t.Fatal(err)
	}
	data, err := store.Load()
	if err != nil || len(data.Projects[0].SecretIDs) != 1 {
		t.Fatal("project activation did not persist")
	}
	cursor, _ := store.Cursor()
	invalid := *project
	invalid.Name = "Must not change"
	invalid.SecretIDs = []string{"missing"}
	if _, err := store.Commit("navigation.updated", project.ID, &invalid); err == nil {
		t.Fatal("missing credential reference was accepted")
	}
	after, _ := store.Cursor()
	data, _ = store.Load()
	if cursor != after || data.Projects[0].Name != project.Name || data.Projects[0].SecretIDs[0] != secret.ID {
		t.Fatal("invalid grant update did not roll back atomically")
	}
	var version int
	if err := store.db.QueryRow("SELECT max(version) FROM schema_migrations").Scan(&version); err != nil || version != 10 {
		t.Fatal("schema six did not migrate")
	}
}
