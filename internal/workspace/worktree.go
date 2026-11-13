// Package workspace owns local Git worktree preparation and identity checks.
package workspace

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var (
	ErrGitMissing  = errors.New("Git is not installed or available on PATH.")
	ErrRepository  = errors.New("The project directory is not a Git repository with a commit.")
	ErrBranch      = errors.New("Enter a valid Git branch name.")
	ErrBase        = errors.New("The base reference does not identify a commit.")
	ErrBranchInUse = errors.New("This branch is checked out in the project directory. Choose another branch.")
	ErrOccupied    = errors.New("The worktree path is already occupied. Its files were preserved.")
	ErrChanged     = errors.New("The branch or repository changed during preparation. Refresh and retry.")
	ErrUnavailable = errors.New("The requirement worktree is unavailable or has a different branch.")
)

type Branch struct {
	Name string `json:"name"`
	Path string `json:"path,omitempty"`
}

type Repository struct {
	Root          string   `json:"root"`
	CommonDir     string   `json:"commonDir"`
	CurrentBranch string   `json:"currentBranch"`
	Branches      []Branch `json:"branches"`
}

// Binding is the durable preparation intent, including the resolved base commit.
// State and Error are managed by the controller; Git never starts agent tasks.
type Binding struct {
	RepositoryRoot string `json:"repositoryRoot"`
	CommonDir      string `json:"commonDir"`
	Branch         string `json:"branch"`
	BaseRef        string `json:"baseRef"`
	BaseCommit     string `json:"baseCommit"`
	Path           string `json:"path"`
	Directory      string `json:"directory"`
	NewBranch      bool   `json:"newBranch"`
	Owned          bool   `json:"owned"`
	State          string `json:"state"`
	Error          string `json:"error,omitempty"`
}

type checkout struct{ path, branch string }

// Version identifies the files currently checked out without requiring a Git project.
func Version(ctx context.Context, directory string) (string, string, error) {
	if _, err := git(ctx, directory, "rev-parse", "--is-inside-work-tree"); err != nil {
		if errors.Is(err, ErrGitMissing) {
			return "", "", nil
		}
		if strings.Contains(err.Error(), "not a git repository") {
			return "", "", nil
		}
		return "", "", err
	}
	reference, _ := git(ctx, directory, "symbolic-ref", "--quiet", "HEAD")
	reference = strings.TrimSpace(reference)
	branch := strings.TrimPrefix(reference, "refs/heads/")
	commit, err := git(ctx, directory, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		if strings.HasPrefix(reference, "refs/heads/") {
			// Only a missing branch reference represents an unborn branch.
			// Retain failures for existing references with unavailable objects.
			refs, refErr := git(ctx, directory, "for-each-ref", "--format=%(refname)", reference)
			refPath, pathErr := git(ctx, directory, "rev-parse", "--git-path", reference)
			refPath = strings.TrimSpace(refPath)
			if !filepath.IsAbs(refPath) {
				refPath = filepath.Join(directory, refPath)
			}
			_, fileErr := os.Lstat(refPath)
			if refErr == nil && pathErr == nil && errors.Is(fileErr, os.ErrNotExist) {
				found := false
				for _, ref := range strings.Fields(refs) {
					found = found || ref == reference
				}
				if !found {
					return branch, "", nil
				}
			}
		}
		return "", "", err
	}
	return strings.TrimSpace(branch), strings.TrimSpace(commit), nil
}

type boundedOutput struct{ bytes.Buffer }

func (b *boundedOutput) Write(p []byte) (int, error) {
	n := len(p)
	if remaining := (1 << 20) - b.Len(); remaining > 0 {
		b.Buffer.Write(p[:min(remaining, n)])
	}
	return n, nil
}

func git(ctx context.Context, directory string, arguments ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	args := append([]string{"-C", directory, "-c", "core.hooksPath=" + os.DevNull}, arguments...)
	command := exec.CommandContext(ctx, "git", args...)
	for _, value := range os.Environ() {
		name, _, _ := strings.Cut(value, "=")
		switch name {
		case "GIT_DIR", "GIT_COMMON_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_OBJECT_DIRECTORY", "GIT_ALTERNATE_OBJECT_DIRECTORIES", "GIT_NAMESPACE":
			continue
		}
		command.Env = append(command.Env, value)
	}
	command.Env = append(command.Env, "GIT_TERMINAL_PROMPT=0")
	var stdout, stderr boundedOutput
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return "", ErrGitMissing
		}
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		message := strings.TrimSpace(stderr.String())
		if len(message) > 2048 {
			message = message[:2048]
		}
		return "", fmt.Errorf("Git operation failed: %s", message)
	}
	return stdout.String(), nil
}

func canonical(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return filepath.Clean(resolved)
	}
	return filepath.Clean(path)
}

func checkouts(ctx context.Context, directory string) ([]checkout, error) {
	raw, err := git(ctx, directory, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return nil, err
	}
	var result []checkout
	var current checkout
	for _, field := range strings.Split(raw, "\x00") {
		switch {
		case strings.HasPrefix(field, "worktree "):
			current = checkout{path: canonical(strings.TrimPrefix(field, "worktree "))}
		case strings.HasPrefix(field, "branch refs/heads/"):
			current.branch = strings.TrimPrefix(field, "branch refs/heads/")
		case field == "" && current.path != "":
			result = append(result, current)
			current = checkout{}
		}
	}
	return result, nil
}

func Inspect(ctx context.Context, directory string) (Repository, error) {
	if err := ctx.Err(); err != nil {
		return Repository{}, err
	}
	root, err := git(ctx, directory, "rev-parse", "--show-toplevel")
	if err != nil {
		if errors.Is(err, ErrGitMissing) {
			return Repository{}, err
		}
		if ctx.Err() != nil {
			return Repository{}, ctx.Err()
		}
		return Repository{}, ErrRepository
	}
	common, err := git(ctx, directory, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return Repository{}, ErrRepository
	}
	if _, err := git(ctx, directory, "rev-parse", "--verify", "HEAD^{commit}"); err != nil {
		return Repository{}, ErrRepository
	}
	root, common = canonical(strings.TrimSpace(root)), canonical(strings.TrimSpace(common))
	current, _ := git(ctx, directory, "symbolic-ref", "--quiet", "--short", "HEAD")
	refs, err := git(ctx, directory, "for-each-ref", "--format=%(refname)", "--sort=refname", "refs/heads/")
	if err != nil {
		return Repository{}, err
	}
	worktrees, err := checkouts(ctx, root)
	if err != nil {
		return Repository{}, err
	}
	result := Repository{Root: root, CommonDir: common, CurrentBranch: strings.TrimSpace(current), Branches: []Branch{}}
	for _, name := range strings.Split(strings.TrimSpace(refs), "\n") {
		if name == "" {
			continue
		}
		name = strings.TrimPrefix(name, "refs/heads/")
		branch := Branch{Name: name}
		for _, tree := range worktrees {
			if tree.branch == name {
				branch.Path = tree.path
			}
		}
		result.Branches = append(result.Branches, branch)
	}
	return result, nil
}

func Plan(ctx context.Context, directory, branch, baseRef, identity string) (Binding, error) {
	if err := ctx.Err(); err != nil {
		return Binding{}, err
	}
	if len(identity) != 32 || strings.Trim(identity, "0123456789abcdef") != "" {
		return Binding{}, ErrOccupied
	}
	branch = strings.TrimSpace(branch)
	if branch == "" {
		branch = "codex/requirement-" + identity[:8]
	}
	if len(branch) > 240 || strings.HasPrefix(branch, "-") || strings.Contains(branch, "@{") {
		return Binding{}, ErrBranch
	}
	repository, err := Inspect(ctx, directory)
	if err != nil {
		return Binding{}, err
	}
	if _, err := git(ctx, directory, "check-ref-format", "--branch", branch); err != nil {
		return Binding{}, ErrBranch
	}
	relative, err := filepath.Rel(repository.Root, canonical(directory))
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return Binding{}, ErrRepository
	}
	key := sha256.Sum256([]byte(repository.CommonDir))
	path := filepath.Join(filepath.Dir(repository.Root), ".loom-worktrees", fmt.Sprintf("%s-%x", filepath.Base(repository.Root), key[:4]), identity)
	plan := Binding{RepositoryRoot: repository.Root, CommonDir: repository.CommonDir, Branch: branch, Path: path, NewBranch: true, Owned: true, State: "preparing"}
	for _, value := range repository.Branches {
		if value.Name != branch {
			continue
		}
		plan.NewBranch = false
		if value.Path != "" {
			if value.Path == repository.Root {
				return Binding{}, ErrBranchInUse
			}
			plan.Path, plan.Owned = value.Path, false
		}
		baseRef = "refs/heads/" + branch
	}
	if baseRef == "" {
		baseRef = "HEAD"
	}
	if len(baseRef) > 512 || strings.HasPrefix(baseRef, "-") || strings.ContainsRune(baseRef, 0) {
		return Binding{}, ErrBase
	}
	commit, err := git(ctx, directory, "rev-parse", "--verify", "--end-of-options", baseRef+"^{commit}")
	if err != nil {
		return Binding{}, ErrBase
	}
	plan.BaseRef, plan.BaseCommit = baseRef, strings.TrimSpace(commit)
	plan.Directory = filepath.Join(plan.Path, relative)
	return plan, nil
}

var repositoryLocks sync.Map

func lock(ctx context.Context, common string) (func(), error) {
	value, _ := repositoryLocks.LoadOrStore(common, make(chan struct{}, 1))
	channel := value.(chan struct{})
	select {
	case channel <- struct{}{}:
		return func() { <-channel }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func Prepare(ctx context.Context, binding Binding) error {
	unlock, err := lock(ctx, binding.CommonDir)
	if err != nil {
		return err
	}
	defer unlock()
	repository, err := Inspect(ctx, binding.RepositoryRoot)
	if err != nil {
		return err
	}
	if repository.CommonDir != binding.CommonDir {
		return ErrChanged
	}
	for _, branch := range repository.Branches {
		if branch.Name == binding.Branch && branch.Path != "" {
			if branch.Path != canonical(binding.Path) {
				return ErrBranchInUse
			}
			return Verify(ctx, binding)
		}
	}
	if !binding.Owned {
		return ErrUnavailable
	}
	if _, err := os.Lstat(binding.Path); !errors.Is(err, os.ErrNotExist) {
		return ErrOccupied
	}
	if err := os.MkdirAll(filepath.Dir(binding.Path), 0755); err != nil {
		return err
	}
	ref := "refs/heads/" + binding.Branch
	commit, branchErr := git(ctx, binding.RepositoryRoot, "rev-parse", "--verify", ref+"^{commit}")
	arguments := []string{"worktree", "add"}
	if branchErr != nil {
		if !binding.NewBranch {
			return ErrChanged
		}
		arguments = append(arguments, "-b", binding.Branch, "--", binding.Path, binding.BaseCommit)
	} else {
		if strings.TrimSpace(commit) != binding.BaseCommit {
			return ErrChanged
		}
		arguments = append(arguments, "--", binding.Path, binding.Branch)
	}
	if _, err := git(ctx, binding.RepositoryRoot, arguments...); err != nil {
		return err
	}
	return Verify(ctx, binding)
}

func Verify(ctx context.Context, binding Binding) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	info, err := os.Stat(binding.Directory)
	if err != nil || !info.IsDir() {
		return ErrUnavailable
	}
	common, err := git(ctx, binding.Directory, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil || canonical(strings.TrimSpace(common)) != binding.CommonDir {
		return ErrUnavailable
	}
	branch, err := git(ctx, binding.Directory, "symbolic-ref", "--quiet", "HEAD")
	if err != nil || strings.TrimSpace(branch) != "refs/heads/"+binding.Branch {
		return ErrUnavailable
	}
	worktrees, err := checkouts(ctx, binding.Directory)
	if err != nil {
		return err
	}
	for _, tree := range worktrees {
		if tree.path == canonical(binding.Path) && tree.branch == binding.Branch {
			return nil
		}
	}
	return ErrUnavailable
}
