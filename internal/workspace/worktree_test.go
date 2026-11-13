package workspace_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/liyiligang/loom/internal/workspace"
)

const identity = "0123456789abcdef0123456789abcdef"

func TestVersionDistinguishesUnbornAndBrokenBranches(t *testing.T) {
	ctx := context.Background()
	directory := t.TempDir()
	command(t, directory, "init", "-b", "main")
	branch, commit, err := workspace.Version(ctx, directory)
	if err != nil || branch != "main" || commit != "" {
		t.Fatalf("unborn version: %q %q %v", branch, commit, err)
	}
	for _, reference := range []string{"broken\n", strings.Repeat("1", 40) + "\n"} {
		if err := os.WriteFile(filepath.Join(directory, ".git/refs/heads/main"), []byte(reference), 0644); err != nil {
			t.Fatal(err)
		}
		if _, _, err := workspace.Version(ctx, directory); err == nil {
			t.Fatal("unavailable branch was accepted as unborn")
		}
	}
}

func command(t *testing.T, directory string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", directory}, args...)...)
	raw, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, raw)
	}
	return strings.TrimSpace(string(raw))
}

func repository(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("Git unavailable")
	}
	path := filepath.Join(t.TempDir(), "project with spaces")
	if err := os.MkdirAll(filepath.Join(path, "app"), 0755); err != nil {
		t.Fatal(err)
	}
	command(t, path, "init", "-b", "main")
	command(t, path, "config", "user.name", "Loom Test")
	command(t, path, "config", "user.email", "loom@example.test")
	if err := os.WriteFile(filepath.Join(path, "app/source.txt"), []byte("Committed"), 0644); err != nil {
		t.Fatal(err)
	}
	command(t, path, "add", ".")
	command(t, path, "commit", "-m", "Initial")
	return path
}

func TestPreparePinsCommittedStateAndPreservesSourceEdits(t *testing.T) {
	root := repository(t)
	ctx := context.Background()
	plan, err := workspace.Plan(ctx, root, "feature/semicolon;safe", "", identity)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "app/source.txt"), []byte("Uncommitted"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "untracked.txt"), []byte("Keep"), 0644); err != nil {
		t.Fatal(err)
	}
	status := command(t, root, "status", "--porcelain")
	if err := workspace.Prepare(ctx, plan); err != nil {
		t.Fatal(err)
	}
	if err := workspace.Prepare(ctx, plan); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if err := workspace.Verify(ctx, plan); err != nil {
		t.Fatal(err)
	}
	if command(t, root, "branch", "--show-current") != "main" || command(t, root, "status", "--porcelain") != status {
		t.Fatal("source branch or edits changed")
	}
	text, _ := os.ReadFile(filepath.Join(plan.Path, "app/source.txt"))
	if string(text) != "Committed" {
		t.Fatal("source edits leaked into the worktree")
	}
	if _, err := os.Stat(filepath.Join(plan.Path, "untracked.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("untracked source file copied")
	}
	if command(t, plan.Path, "rev-parse", "HEAD") != plan.BaseCommit {
		t.Fatal("base was not pinned")
	}
}

func TestExistingBranchAdoptionAndProjectCheckoutRejection(t *testing.T) {
	root := repository(t)
	ctx := context.Background()
	if _, err := workspace.Plan(ctx, root, "main", "", identity); !errors.Is(err, workspace.ErrBranchInUse) {
		t.Fatalf("source checkout accepted: %v", err)
	}
	command(t, root, "branch", "feature/existing")
	command(t, root, "tag", "feature/existing")
	plan, err := workspace.Plan(ctx, root, "feature/existing", "", identity)
	if err != nil || plan.NewBranch {
		t.Fatalf("existing branch: %+v, %v", plan, err)
	}
	if err := workspace.Prepare(ctx, plan); err != nil {
		t.Fatal(err)
	}
	adopted, err := workspace.Plan(ctx, root, plan.Branch, "", "abcdef0123456789abcdef0123456789")
	if err != nil || adopted.Owned || adopted.Path != plan.Path {
		t.Fatalf("adoption: %+v, %v", adopted, err)
	}
	if err := workspace.Prepare(ctx, adopted); err != nil {
		t.Fatal(err)
	}
	command(t, plan.Path, "switch", "-c", "feature/changed")
	if err := workspace.Verify(ctx, plan); !errors.Is(err, workspace.ErrUnavailable) {
		t.Fatalf("branch replacement accepted: %v", err)
	}
}

func TestOccupiedPathsAndMovedBranchesArePreserved(t *testing.T) {
	root := repository(t)
	ctx := context.Background()
	plan, _ := workspace.Plan(ctx, root, "feature/occupied", "", identity)
	if err := os.MkdirAll(plan.Path, 0755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(plan.Path, "keep.txt")
	if err := os.WriteFile(marker, []byte("Keep"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := workspace.Prepare(ctx, plan); !errors.Is(err, workspace.ErrOccupied) {
		t.Fatalf("occupied path: %v", err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatal("occupied data removed")
	}
	command(t, root, "branch", "feature/moved")
	moved, _ := workspace.Plan(ctx, root, "feature/moved", "", "abcdef0123456789abcdef0123456789")
	command(t, root, "commit", "--allow-empty", "-m", "Advance")
	command(t, root, "branch", "-f", "feature/moved", "HEAD")
	if err := workspace.Prepare(ctx, moved); !errors.Is(err, workspace.ErrChanged) {
		t.Fatalf("changed existing branch accepted: %v", err)
	}
}

func TestSubdirectoryRecoveryConcurrentRetriesAndHookIsolation(t *testing.T) {
	root := repository(t)
	ctx := context.Background()
	plan, err := workspace.Plan(ctx, filepath.Join(root, "app"), "", "main", identity)
	if err != nil || plan.Directory != filepath.Join(plan.Path, "app") {
		t.Fatalf("subdirectory: %+v, %v", plan, err)
	}
	// Simulate a failed checkout after Git created the requested branch.
	command(t, root, "branch", plan.Branch, plan.BaseCommit)
	hook := filepath.Join(root, ".git/hooks/post-checkout")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\ntouch hook-ran.txt\n"), 0755); err != nil {
		t.Fatal(err)
	}
	command(t, root, "commit", "--allow-empty", "-m", "Move base after intent")
	var workers sync.WaitGroup
	for range 4 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			if err := workspace.Prepare(ctx, plan); err != nil {
				t.Errorf("retry: %v", err)
			}
		}()
	}
	workers.Wait()
	if command(t, plan.Path, "rev-parse", "HEAD") != plan.BaseCommit {
		t.Fatal("retry used a moved base")
	}
	if _, err := os.Stat(filepath.Join(plan.Path, "hook-ran.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("checkout hook ran")
	}
}

func TestInvalidInputCancellationAndGitEnvironmentIsolation(t *testing.T) {
	root := repository(t)
	for _, branch := range []string{"-bad", "HEAD", "bad name", "@{-1}", "a..b"} {
		if _, err := workspace.Plan(context.Background(), root, branch, "", identity); !errors.Is(err, workspace.ErrBranch) {
			t.Errorf("invalid branch %q: %v", branch, err)
		}
	}
	if _, err := workspace.Plan(context.Background(), root, "feature/base", "missing", identity); !errors.Is(err, workspace.ErrBase) {
		t.Fatalf("invalid base: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := workspace.Plan(ctx, root, "feature/cancel", "", identity); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
	t.Setenv("GIT_DIR", "/unrelated/repository.git")
	t.Setenv("GIT_WORK_TREE", "/unrelated/project")
	plan, err := workspace.Plan(context.Background(), root, "feature/env", "", identity)
	if err != nil {
		t.Fatal(err)
	}
	if err := workspace.Prepare(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
}
