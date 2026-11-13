package workspace

import (
	"context"
	"strings"
)

type GroupPlan struct {
	Prefix         string
	RepositoryRoot string
	CommonDir      string
	BaseRef        string
	BaseCommit     string
}

// PlanGroup validates a namespace and pins its base without creating a Git ref.
func PlanGroup(ctx context.Context, directory, prefix, baseRef string) (GroupPlan, error) {
	prefix = strings.TrimRight(strings.TrimSpace(prefix), "/")
	if prefix == "" || len(prefix) > 200 || strings.HasPrefix(prefix, "-") || strings.Contains(prefix, "@{") {
		return GroupPlan{}, ErrBranch
	}
	repository, err := Inspect(ctx, directory)
	if err != nil {
		return GroupPlan{}, err
	}
	if _, err := git(ctx, directory, "check-ref-format", "--branch", prefix+"/member"); err != nil {
		return GroupPlan{}, ErrBranch
	}
	for _, branch := range repository.Branches {
		if prefix == branch.Name || strings.HasPrefix(prefix, branch.Name+"/") {
			return GroupPlan{}, ErrBranchInUse
		}
	}
	baseRef = strings.TrimSpace(baseRef)
	if baseRef == "" {
		baseRef = "HEAD"
	}
	if len(baseRef) > 512 || strings.HasPrefix(baseRef, "-") || strings.ContainsRune(baseRef, 0) {
		return GroupPlan{}, ErrBase
	}
	commit, err := git(ctx, directory, "rev-parse", "--verify", "--end-of-options", baseRef+"^{commit}")
	if err != nil {
		return GroupPlan{}, ErrBase
	}
	return GroupPlan{Prefix: prefix, RepositoryRoot: repository.Root, CommonDir: repository.CommonDir,
		BaseRef: baseRef, BaseCommit: strings.TrimSpace(commit)}, nil
}
