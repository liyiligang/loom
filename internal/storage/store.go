package storage

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/liyiligang/loom/internal/model"
	_ "modernc.org/sqlite"
)

//go:embed migrations/001_initial.sql
var initialMigration string

//go:embed migrations/002_local_setup.sql
var localMigration string

//go:embed migrations/003_navigation.sql
var navigationMigration string

//go:embed migrations/004_requirement_workspaces.sql
var workspaceMigration string

//go:embed migrations/005_workspace_history.sql
var workspaceHistoryMigration string

//go:embed migrations/006_secrets.sql
var secretsMigration string

//go:embed migrations/007_project_secrets.sql
var projectSecretsMigration string

//go:embed migrations/008_requirement_materials.sql
var materialsMigration string

//go:embed migrations/009_branch_groups.sql
var branchGroupsMigration string

//go:embed migrations/010_requirement_workflows.sql
var workflowsMigration string

type Store struct {
	db   *sql.DB
	path string
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db, path: path}
	if err := s.initialize(); err != nil {
		db.Close()
		return nil, fmt.Errorf("initialize database: %w", err)
	}
	return s, nil
}

func (s *Store) initialize() error {
	if _, err := s.db.Exec("PRAGMA foreign_keys = ON; PRAGMA journal_mode = WAL; PRAGMA busy_timeout = 5000;"); err != nil {
		return err
	}
	var exists int
	if err := s.db.QueryRow("SELECT count(*) FROM sqlite_master WHERE name = 'schema_migrations'").Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if _, err := tx.Exec(initialMigration); err != nil {
			return err
		}
		if _, err := tx.Exec(localMigration); err != nil {
			return err
		}
		if _, err := tx.Exec(navigationMigration); err != nil {
			return err
		}
		if _, err := tx.Exec(workspaceMigration); err != nil {
			return err
		}
		if _, err := tx.Exec(workspaceHistoryMigration); err != nil {
			return err
		}
		if _, err := tx.Exec(secretsMigration); err != nil {
			return err
		}
		if _, err := tx.Exec(projectSecretsMigration); err != nil {
			return err
		}
		if _, err := tx.Exec(materialsMigration); err != nil {
			return err
		}
		if _, err := tx.Exec(branchGroupsMigration); err != nil {
			return err
		}
		if err := migrateBranchGroups(tx); err != nil {
			return err
		}
		if _, err := tx.Exec(workflowsMigration); err != nil {
			return err
		}
		return tx.Commit()
	}
	var version int
	if err := s.db.QueryRow("SELECT max(version) FROM schema_migrations").Scan(&version); err != nil {
		return err
	}
	if version < 1 || version > 10 {
		return fmt.Errorf("unsupported schema version %d", version)
	}
	if version < 10 {
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if version == 1 {
			if _, err := tx.Exec(localMigration); err != nil {
				return err
			}
		}
		if version < 3 {
			if _, err := tx.Exec(navigationMigration); err != nil {
				return err
			}
		}
		if version < 4 {
			if _, err := tx.Exec(workspaceMigration); err != nil {
				return err
			}
		}
		if version < 5 {
			if _, err := tx.Exec(workspaceHistoryMigration); err != nil {
				return err
			}
		}
		if version < 6 {
			if _, err := tx.Exec(secretsMigration); err != nil {
				return err
			}
		}
		if version < 7 {
			if _, err := tx.Exec(projectSecretsMigration); err != nil {
				return err
			}
		}
		if version < 8 {
			if _, err := tx.Exec(materialsMigration); err != nil {
				return err
			}
		}
		if version < 9 {
			if _, err := tx.Exec(branchGroupsMigration); err != nil {
				return err
			}
			if err := migrateBranchGroups(tx); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(workflowsMigration); err != nil {
			return err
		}
		return tx.Commit()
	}
	return nil
}

func load[T any](db *sql.DB, table string) ([]*T, error) {
	rows, err := db.Query("SELECT data FROM " + table + " ORDER BY rowid")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]*T, 0)
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var value T
		if err := json.Unmarshal(raw, &value); err != nil {
			return nil, err
		}
		result = append(result, &value)
	}
	return result, rows.Err()
}

func (s *Store) Load() (model.Data, error) {
	var data model.Data
	var err error
	if data.Workflows, err = load[model.RequirementWorkflow](s.db, "requirement_workflows"); err != nil {
		return data, err
	}
	data.LocalSetup = &model.LocalSetup{}
	data.ModelCatalog = &model.ModelCatalog{Models: []model.ConfiguredModel{}}
	var catalog []byte
	if err = s.db.QueryRow("SELECT data FROM settings WHERE id = 'ai_models'").Scan(&catalog); err == nil {
		if err = json.Unmarshal(catalog, data.ModelCatalog); err != nil {
			return data, err
		}
	} else if err != sql.ErrNoRows {
		return data, err
	}
	if data.Secrets, err = load[model.Secret](s.db, "secrets"); err != nil {
		return data, err
	}
	var setup []byte
	if err = s.db.QueryRow("SELECT data FROM settings WHERE id = 'local_setup'").Scan(&setup); err == nil {
		if err = json.Unmarshal(setup, data.LocalSetup); err != nil {
			return data, err
		}
	} else if err != sql.ErrNoRows {
		return data, err
	}
	if data.Endpoints, err = load[model.Endpoint](s.db, "endpoints"); err != nil {
		return data, err
	}
	if data.Projects, err = load[model.Project](s.db, "projects"); err != nil {
		return data, err
	}
	if data.Requirements, err = load[model.Requirement](s.db, "requirements"); err != nil {
		return data, err
	}
	if data.Sessions, err = load[model.Session](s.db, "sessions"); err != nil {
		return data, err
	}
	if data.Turns, err = load[model.Turn](s.db, "turns"); err != nil {
		return data, err
	}
	if data.Entries, err = load[model.Entry](s.db, "entries"); err != nil {
		return data, err
	}
	records, err := load[model.InteractionRecord](s.db, "interactions")
	if err != nil {
		return data, err
	}
	for _, record := range records {
		record.RequestID, record.Generation = record.NativeID, record.Connection
		data.Interactions = append(data.Interactions, &record.Interaction)
	}
	return data, nil
}

// Commit atomically saves projections and their notification cursor.
func (s *Store) Commit(eventType, resource string, values ...any) (int64, error) {
	tx, err := s.db.BeginTx(context.Background(), nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	for _, value := range values {
		if err := save(tx, value); err != nil {
			return 0, err
		}
	}
	result, err := tx.Exec("INSERT INTO events(type, resource, created_at) VALUES (?, ?, ?)", eventType, resource, model.Now())
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

func save(tx *sql.Tx, value any) error {
	if removal, ok := value.(*model.SecretRemoval); ok {
		_, err := tx.Exec("DELETE FROM secrets WHERE id = ?", removal.ID)
		return err
	}
	if requirement, ok := value.(*model.Requirement); ok {
		requirement.SyncBranchGroup()
		if group := requirement.BranchGroup; group != nil {
			if group.ID == "" || group.CommonDir == "" || group.Prefix == "" {
				return fmt.Errorf("invalid branch group identity")
			}
			seen := make(map[string]bool)
			for _, member := range group.Members {
				if member.ID == "" || seen[member.ID] {
					return fmt.Errorf("invalid branch group member identity")
				}
				seen[member.ID] = true
			}
			if group.MaterialBranchID != "" && group.Member(group.MaterialBranchID) == nil {
				return fmt.Errorf("invalid branch group material source")
			}
		}
	}
	stored := value
	if interaction, ok := value.(*model.Interaction); ok {
		stored = model.InteractionRecord{Interaction: *interaction, NativeID: interaction.RequestID, Connection: interaction.Generation}
	}
	raw, err := json.Marshal(stored)
	if err != nil {
		return err
	}
	var query string
	var args []any
	switch v := value.(type) {
	case *model.RequirementWorkflow:
		query = "INSERT INTO requirement_workflows VALUES (?, ?, ?) ON CONFLICT(requirement_id) DO UPDATE SET id=excluded.id, data=excluded.data"
		args = []any{v.ID, v.RequirementID, raw}
	case *model.SecretRecord:
		query = "INSERT INTO secrets VALUES (?, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET name=excluded.name, data=excluded.data, ciphertext=excluded.ciphertext"
		args = []any{v.ID, v.Name, raw, v.Ciphertext}
	case *model.LocalSetup:
		query = "INSERT INTO settings VALUES (?, ?) ON CONFLICT(id) DO UPDATE SET data=excluded.data"
		args = []any{"local_setup", raw}
	case *model.ModelCatalog:
		query = "INSERT INTO settings VALUES (?, ?) ON CONFLICT(id) DO UPDATE SET data=excluded.data"
		args = []any{"ai_models", raw}
	case *model.Endpoint:
		query = "INSERT INTO endpoints VALUES (?, ?) ON CONFLICT(id) DO UPDATE SET data=excluded.data"
		args = []any{v.ID, raw}
	case *model.Project:
		scope := "local"
		if !v.Local {
			scope = "endpoint:" + v.EndpointID
		}
		query = "INSERT INTO projects VALUES (?, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET data=excluded.data"
		args = []any{v.ID, scope, v.Cwd, raw}
	case *model.Requirement:
		query = "INSERT INTO requirements VALUES (?, ?, ?) ON CONFLICT(id) DO UPDATE SET data=excluded.data"
		args = []any{v.ID, v.ProjectID, raw}
	case *model.Session:
		var requirement any
		if v.RequirementID != "" {
			requirement = v.RequirementID
		}
		query = "INSERT INTO sessions(id, endpoint_id, provider_id, data, requirement_id) VALUES (?, ?, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET data=excluded.data, requirement_id=excluded.requirement_id"
		args = []any{v.ID, v.EndpointID, v.ProviderID, raw, requirement}
	case *model.Turn:
		query = "INSERT INTO turns VALUES (?, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET data=excluded.data"
		args = []any{v.ID, v.SessionID, v.ClientRequestID, raw}
	case *model.Entry:
		query = "INSERT INTO entries VALUES (?, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET data=excluded.data"
		args = []any{v.ID, v.SessionID, v.TurnID, raw}
	case *model.Interaction:
		query = "INSERT INTO interactions VALUES (?, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET data=excluded.data"
		args = []any{v.ID, v.SessionID, v.TurnID, raw}
	default:
		return fmt.Errorf("unsupported projection %T", value)
	}
	if _, err := tx.Exec(query, args...); err != nil {
		return err
	}
	if project, ok := value.(*model.Project); ok {
		if _, err := tx.Exec("DELETE FROM project_secrets WHERE project_id = ?", project.ID); err != nil {
			return err
		}
		for _, id := range project.SecretIDs {
			if _, err := tx.Exec("INSERT INTO project_secrets VALUES (?, ?)", project.ID, id); err != nil {
				return err
			}
		}
	}
	if requirement, ok := value.(*model.Requirement); ok {
		if _, err := tx.Exec("DELETE FROM requirement_branch_groups WHERE requirement_id = ?", requirement.ID); err != nil {
			return err
		}
		if group := requirement.BranchGroup; group != nil {
			if _, err := tx.Exec("INSERT INTO requirement_branch_groups VALUES (?, ?, ?, ?)", group.ID, requirement.ID, group.CommonDir, group.Prefix); err != nil {
				return err
			}
		}
		if _, err := tx.Exec("DELETE FROM requirement_workspace_branches WHERE requirement_id = ?", requirement.ID); err != nil {
			return err
		}
		for _, binding := range requirement.WorkspaceBindings() {
			if _, err := tx.Exec("INSERT INTO requirement_workspace_branches VALUES (?, ?, ?)", binding.CommonDir, binding.Branch, requirement.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Store) Cursor() (int64, error) {
	var id int64
	err := s.db.QueryRow("SELECT coalesce(max(id), 0) FROM events").Scan(&id)
	return id, err
}

func (s *Store) Events(after int64) ([]model.Event, error) {
	rows, err := s.db.Query("SELECT id, type, resource, created_at FROM events WHERE id > ? ORDER BY id LIMIT 100", after)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]model.Event, 0)
	for rows.Next() {
		var event model.Event
		if err := rows.Scan(&event.ID, &event.Type, &event.Resource, &event.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, event)
	}
	return result, rows.Err()
}

func (s *Store) Directory() string {
	absolute, _ := filepath.Abs(filepath.Dir(s.path))
	return absolute
}

func (s *Store) Close() error { return s.db.Close() }
