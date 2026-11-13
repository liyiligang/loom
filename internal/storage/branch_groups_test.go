package storage

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/liyiligang/loom/internal/model"
	"github.com/liyiligang/loom/internal/workspace"
)

func schemaEight(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, migration := range []string{initialMigration, localMigration, navigationMigration, workspaceMigration, workspaceHistoryMigration, secretsMigration, projectSecretsMigration, materialsMigration} {
		if _, err := db.Exec(migration); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func TestSchemaEightMigratesCurrentAndRetainedBranchesWithoutChangingHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "loom.db")
	legacy := schemaEight(t, path)
	current := workspace.Binding{RepositoryRoot: "/repo", CommonDir: "/repo/.git", Branch: "feature/current", BaseRef: "main", BaseCommit: "pinned", Path: "/trees/current", Directory: "/trees/current", State: "ready"}
	retained := current
	retained.Branch, retained.Path, retained.Directory = "feature/retained", "/trees/retained", "/trees/retained"
	requirement := model.Requirement{ID: "requirement", ProjectID: "project", Name: "Saved objective", Workspace: &current, PreviousWorkspaces: []workspace.Binding{retained}}
	raw, _ := json.Marshal(requirement)
	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO endpoints VALUES ('agent', '{"id":"agent","provider":"codex"}')`, nil},
		{`INSERT INTO projects VALUES ('project','local','/repo','{"id":"project","cwd":"/repo","local":true}')`, nil},
		{`INSERT INTO requirements VALUES ('requirement','project',?)`, []any{raw}},
		{`INSERT INTO requirement_workspace_branches VALUES ('/repo/.git','feature/current','requirement')`, nil},
		{`INSERT INTO requirement_workspace_branches VALUES ('/repo/.git','feature/retained','requirement')`, nil},
		{`INSERT INTO settings VALUES ('local_setup','{"confirmed":true}')`, nil},
	}
	for _, statement := range statements {
		if _, err := legacy.Exec(statement.query, statement.args...); err != nil {
			t.Fatal(err)
		}
	}
	for index, binding := range []*workspace.Binding{&current, &retained, nil} {
		id := []string{"current", "retained", "root"}[index]
		session := model.Session{ID: id, RequirementID: requirement.ID, EndpointID: "agent", ProviderID: "native-" + id, Cwd: "/repo", Workspace: binding, State: "idle", Title: "Saved discussion"}
		if binding != nil {
			session.Cwd = binding.Directory
		}
		raw, _ := json.Marshal(session)
		if _, err := legacy.Exec("INSERT INTO sessions VALUES (?,?,?,?,?)", id, "agent", session.ProviderID, raw, requirement.ID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := legacy.Exec(`INSERT INTO turns VALUES ('turn','current','request','{"id":"turn","sessionId":"current","clientRequestId":"request","state":"completed","input":"Saved task"}')`); err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.Exec(`INSERT INTO entries VALUES ('entry','current','turn','{"id":"entry","sessionId":"current","turnId":"turn","text":"Saved result"}')`); err != nil {
		t.Fatal(err)
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
	r := data.Requirements[0]
	group := r.BranchGroup
	if group == nil || group.ID != r.ID || len(group.Members) != 2 || group.ExplicitSelection || group.Member(group.MaterialBranchID).Workspace.Branch != current.Branch || *r.Workspace != current || r.PreviousWorkspaces[0] != retained {
		t.Fatal("migration changed branch bindings or lost group membership")
	}
	for _, session := range data.Sessions {
		if session.ProviderID != "native-"+session.ID || session.Title != "Saved discussion" {
			t.Fatal("migration changed native conversation metadata")
		}
		if session.Workspace != nil && group.Member(session.BranchID).Workspace.Branch != session.Workspace.Branch {
			t.Fatal("migration lost a session's member association")
		}
		if session.Workspace == nil && session.BranchID != "" {
			t.Fatal("migration repointed a project-root conversation")
		}
	}
	if !data.LocalSetup.Confirmed || data.Entries[0].Text != "Saved result" || data.Turns[0].Input != "Saved task" {
		t.Fatal("migration changed saved history or consent")
	}
	var groups, branches, version int
	store.db.QueryRow("SELECT count(*) FROM requirement_branch_groups").Scan(&groups)
	store.db.QueryRow("SELECT count(*) FROM requirement_workspace_branches").Scan(&branches)
	store.db.QueryRow("SELECT max(version) FROM schema_migrations").Scan(&version)
	if groups != 1 || branches != 2 || version != 10 {
		t.Fatal("migration did not preserve relational ownership")
	}
}

func TestBranchGroupIndexConflictRollsBackProjectionAndEvent(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "loom.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	project := &model.Project{ID: "project", Local: true, Cwd: "/repo"}
	first := &model.Requirement{ID: "first", ProjectID: project.ID, Name: "First", BranchGroup: &model.BranchGroup{ID: "group-one", Prefix: "feat/console", CommonDir: "/repo/.git", Members: []model.BranchMember{}, ExplicitSelection: true}}
	if _, err := store.Commit("navigation.updated", first.ID, project, first); err != nil {
		t.Fatal(err)
	}
	before, _ := store.Cursor()
	second := &model.Requirement{ID: "second", ProjectID: project.ID, Name: "Second", BranchGroup: &model.BranchGroup{ID: "group-two", Prefix: "feat/console", CommonDir: "/repo/.git", Members: []model.BranchMember{}, ExplicitSelection: true}}
	if _, err := store.Commit("navigation.updated", second.ID, second); err == nil {
		t.Fatal("duplicate group prefix was accepted")
	}
	after, _ := store.Cursor()
	data, err := store.Load()
	if err != nil || before != after || len(data.Requirements) != 1 || data.Requirements[0].BranchGroup.ID != "group-one" {
		t.Fatal("failed group transaction partially persisted")
	}
}
