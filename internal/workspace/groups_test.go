package workspace_test

import (
	"context"
	"testing"

	"github.com/liyiligang/loom/internal/workspace"
)

func TestBranchGroupPlanningDoesNotCreateRefsOrWorktrees(t *testing.T) {
	directory := repository(t)
	before := command(t, directory, "worktree", "list", "--porcelain")
	plan, err := workspace.PlanGroup(context.Background(), directory, "feat/console/", "main")
	if err != nil || plan.Prefix != "feat/console" || plan.BaseCommit != command(t, directory, "rev-parse", "HEAD") {
		t.Fatalf("group plan: %+v %v", plan, err)
	}
	if command(t, directory, "branch", "--show-current") != "main" || command(t, directory, "worktree", "list", "--porcelain") != before || command(t, directory, "branch", "--list", "feat/console*") != "" {
		t.Fatal("planning a group changed Git state")
	}
	for _, prefix := range []string{"", "bad prefix", "feat/../escape", "main", "main/child", "-option", "feat/@{bad}"} {
		if _, err := workspace.PlanGroup(context.Background(), directory, prefix, "main"); err == nil {
			t.Errorf("accepted unavailable prefix %q", prefix)
		}
	}
}
