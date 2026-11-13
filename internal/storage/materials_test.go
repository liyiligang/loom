package storage

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/liyiligang/loom/internal/materials"
	"github.com/liyiligang/loom/internal/model"
)

func TestSchemaSevenAddsMetadataIndexAndPreservesRelationships(t *testing.T) {
	path := filepath.Join(t.TempDir(), "loom.db")
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, migration := range []string{initialMigration, localMigration, navigationMigration, workspaceMigration, workspaceHistoryMigration, secretsMigration, projectSecretsMigration} {
		if _, err := legacy.Exec(migration); err != nil {
			t.Fatal(err)
		}
	}
	legacy.Close()
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	project := &model.Project{ID: "p", Local: true, Cwd: "/repo"}
	requirement := &model.Requirement{ID: "r", ProjectID: project.ID, Name: "Requirement"}
	if _, err := store.Commit("navigation.updated", "r", project, requirement); err != nil {
		t.Fatal(err)
	}
	scope := materials.NewScope("p", "r", "/repo", "main", "one")
	catalog := materials.Catalog{Scope: scope, Files: []materials.File{{ID: "doc", Path: "documents/a.md", Kind: "documents", Hash: "hash"}, {ID: "test", Path: "test-cases/a.yaml", Kind: "testCases", Hash: "testhash"}}}
	if err := store.SyncMaterials(&catalog); err != nil {
		t.Fatal(err)
	}
	if err := store.SetMaterialLinks(scope, "doc", []string{"test"}); err != nil {
		t.Fatal(err)
	}
	if err := store.SetMaterialLinks(scope, "doc", []string{"missing"}); err == nil {
		t.Fatal("missing target was accepted")
	}
	other := catalog
	other.Scope = materials.NewScope("p", "r", "/other", "feature", "two")
	if err := store.SyncMaterials(&other); err != nil {
		t.Fatal(err)
	}
	store.Close()
	store, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	catalog.Files[0].Hash = "newhash"
	if err := store.SyncMaterials(&catalog); err != nil {
		t.Fatal(err)
	}
	if len(catalog.Files[0].RelatedIDs) != 1 || catalog.Files[0].RelatedIDs[0] != "test" {
		t.Fatal("restart, upsert, or invalid-link rollback lost relationships")
	}
	var count int
	if err := store.db.QueryRow("SELECT count(*) FROM pragma_table_info('requirement_materials') WHERE name IN ('content','body')").Scan(&count); err != nil || count != 0 {
		t.Fatal("index stores document bodies")
	}
	partial := materials.Catalog{Scope: scope, Files: catalog.Files[:1], Limited: true}
	if err := store.SyncMaterials(&partial); err != nil {
		t.Fatal(err)
	}
	if len(partial.Files[0].RelatedIDs) != 1 {
		t.Fatal("partial scan removed unscanned relationships")
	}
	partial.Limited = false
	if err := store.SyncMaterials(&partial); err != nil {
		t.Fatal(err)
	}
	if len(partial.Files[0].RelatedIDs) != 0 {
		t.Fatal("deleted target retained dangling relationships")
	}
	if err := store.db.QueryRow("SELECT count(*) FROM requirement_materials WHERE scope=?", other.Scope.Key).Scan(&count); err != nil || count != 2 {
		t.Fatal("refresh removed another branch's index")
	}
	if _, err := store.db.Exec("DELETE FROM requirement_materials WHERE requirement_id=? AND scope=?", "r", scope.Key); err != nil {
		t.Fatal(err)
	}
	if err := store.SyncMaterials(&catalog); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow("SELECT count(*) FROM requirement_materials WHERE scope=?", scope.Key).Scan(&count); err != nil || count != 2 {
		t.Fatal("metadata could not be rebuilt from a file scan")
	}
}
