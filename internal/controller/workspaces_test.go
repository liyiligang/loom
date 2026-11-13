package controller

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/liyiligang/loom/internal/model"
	"github.com/liyiligang/loom/internal/storage"
	"github.com/liyiligang/loom/internal/workspace"
)

func TestPreparingBindingAllowsRenameButRejectsReplacement(t *testing.T) {
	store, err := storage.Open(filepath.Join(t.TempDir(), "loom.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	project := &model.Project{ID: "p", Local: true, Cwd: "/repo"}
	requirement := &model.Requirement{ID: "r", ProjectID: project.ID, Name: "Feature", Workspace: &workspace.Binding{CommonDir: "/repo/.git", Branch: "feature/preparing", State: "preparing"}}
	if _, err := store.Commit("navigation.updated", requirement.ID, project, requirement, &model.LocalSetup{Confirmed: true}); err != nil {
		t.Fatal(err)
	}
	service, err := New(context.Background(), store, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	branch, name := "feature/replacement", "Renamed"
	if err := service.UpdateRequirement(context.Background(), requirement.ID, RequirementUpdate{Branch: &branch, Name: &name}); err == nil {
		t.Fatal("an outstanding preparation intent was replaced")
	}
	if err := service.UpdateRequirement(context.Background(), requirement.ID, RequirementUpdate{Name: &name}); err != nil {
		t.Fatal(err)
	}
	data, err := store.Load()
	if err != nil || data.Requirements[0].Name != name || data.Requirements[0].Workspace.Branch != requirement.Workspace.Branch || len(data.Requirements[0].PreviousWorkspaces) != 0 {
		t.Fatal("rename changed preparation intent")
	}
}
