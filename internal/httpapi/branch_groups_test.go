package httpapi_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/liyiligang/loom/internal/materials"
	"github.com/liyiligang/loom/internal/model"
)

func (f *fixture) requirementSnapshot(id string) model.Requirement {
	f.t.Helper()
	var navigation navigation
	f.call("GET", "/bootstrap", nil, 200, &navigation)
	for _, requirement := range navigation.Requirements {
		if requirement.ID == id {
			return requirement
		}
	}
	f.t.Fatal("requirement missing from bootstrap")
	return model.Requirement{}
}

func TestBranchGroupRetryKeepsIdentityAndCreationBlocksGroupEdits(t *testing.T) {
	f, id, _ := materialFixture(t)
	route := "/requirements/" + id
	f.call("PUT", route+"/branch-group", map[string]any{"prefix": "feat/recovery"}, 200, nil)
	var member model.BranchMember
	f.call("POST", route+"/branches", map[string]any{"name": "implement"}, 201, &member)
	member = f.waitBranchMember(id, member.ID, "ready")
	gitCommand(t, member.Workspace.Directory, "switch", "-c", "external/change")
	f.call("POST", route+"/branches/"+member.ID+"/retry", nil, 200, nil)
	f.waitBranchMember(id, member.ID, "error")
	f.call("POST", "/sessions", map[string]any{"endpointId": f.endpoint, "requirementId": id, "branchId": member.ID}, 409, nil)
	gitCommand(t, member.Workspace.Directory, "switch", member.Workspace.Branch)
	f.call("POST", route+"/branches/"+member.ID+"/retry", nil, 200, nil)
	recovered := f.waitBranchMember(id, member.ID, "ready")
	if recovered.Workspace.Path != member.Workspace.Path || recovered.ID != member.ID {
		t.Fatal("retry replaced the member or worktree")
	}
	started, release := make(chan struct{}, 1), make(chan struct{})
	f.native.mu.Lock()
	f.native.createStarted, f.native.createRelease = started, release
	f.native.mu.Unlock()
	result := make(chan struct{ ID string }, 1)
	go func() {
		var session struct{ ID string }
		f.call("POST", "/sessions", map[string]any{"endpointId": f.endpoint, "requirementId": id, "branchId": member.ID}, 201, &session)
		result <- session
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		close(release)
		t.Fatal("native session creation did not start")
	}
	f.call("PUT", route+"/branch-group", map[string]any{"prefix": "feat/changed"}, 409, nil)
	f.call("PUT", route+"/branch-group/materials", map[string]any{"branchId": member.ID}, 409, nil)
	close(release)
	session := <-result
	if f.requirementSnapshot(id).BranchGroup.Prefix != "feat/recovery" || f.snapshot(session.ID).Session.BranchID != member.ID {
		t.Fatal("creation race changed group selection or session identity")
	}
}

func (f *fixture) waitBranchMember(id, memberID, state string) model.BranchMember {
	f.t.Helper()
	var result model.BranchMember
	eventually(f.t, func() bool {
		requirement := f.requirementSnapshot(id)
		member := requirement.BranchGroup.Member(memberID)
		if member == nil {
			return false
		}
		result = *member
		return result.Workspace.State == state
	})
	return result
}

func TestBranchGroupBindingAndNamespaceOwnership(t *testing.T) {
	f, id, directory := materialFixture(t)
	before := gitCommand(t, directory, "worktree", "list", "--porcelain")
	for _, prefix := range []string{"", "bad prefix", "main", "main/child", "feat/../escape"} {
		f.call("PUT", "/requirements/"+id+"/branch-group", map[string]any{"prefix": prefix}, 400, nil)
	}
	f.call("PUT", "/requirements/"+id+"/branch-group", map[string]any{"prefix": "feat/console/", "baseRef": "main"}, 200, nil)
	first := f.requirementSnapshot(id)
	if first.BranchGroup == nil || first.BranchGroup.Prefix != "feat/console" || len(first.BranchGroup.Members) != 0 || first.Workspace != nil || !first.BranchGroup.ExplicitSelection {
		t.Fatal("binding a group created or selected a concrete branch")
	}
	if gitCommand(t, directory, "worktree", "list", "--porcelain") != before || gitCommand(t, directory, "branch", "--list", "feat/console*") != "" {
		t.Fatal("binding a group changed Git state")
	}
	f.call("POST", "/sessions", map[string]any{"endpointId": f.endpoint, "requirementId": id}, 400, nil)
	var second struct{ ID string }
	f.call("POST", "/requirements", map[string]any{"projectId": first.ProjectID, "name": "Other"}, 201, &second)
	for _, prefix := range []string{"feat/console", "feat/console/nested", "feat"} {
		f.call("PUT", "/requirements/"+second.ID+"/branch-group", map[string]any{"prefix": prefix}, 409, nil)
	}
	if f.requirementSnapshot(second.ID).BranchGroup != nil {
		t.Fatal("rejected group prefix partially persisted")
	}
	f.call("POST", "/requirements", map[string]any{"projectId": first.ProjectID, "name": "Legacy collision", "branch": "feat/console/claimed"}, 409, nil)
	f.call("PUT", "/requirements/"+second.ID+"/branch-group", map[string]any{"prefix": "feat/other"}, 200, nil)
	f.native.mu.Lock()
	defer f.native.mu.Unlock()
	if len(f.native.threads) != 0 || f.native.starts != 0 {
		t.Fatal("group management started an agent conversation")
	}
}

func TestBranchGroupMembersKeepSessionsAndDocumentSourceIndependent(t *testing.T) {
	f, id, directory := materialFixture(t)
	route := "/requirements/" + id
	var catalog materials.Catalog
	f.call("GET", route+"/materials", nil, 200, &catalog)
	materialDirectory := catalog.Scope.Directory
	var document materials.Detail
	f.call("POST", route+"/materials", map[string]any{"kind": "documents", "name": "requirements.md", "content": "# Shared acceptance\n", "scopeKey": catalog.Scope.Key}, 201, &document)
	f.call("PUT", route+"/branch-group", map[string]any{"prefix": "feat/console"}, 200, nil)
	group := f.requirementSnapshot(id).BranchGroup
	// Later project commits do not change the group's pinned base.
	if err := os.WriteFile(filepath.Join(directory, "later.txt"), []byte("Later project change"), 0644); err != nil {
		t.Fatal(err)
	}
	gitCommand(t, directory, "add", "later.txt")
	gitCommand(t, directory, "commit", "-m", "Later project change")
	var implementation, optimization, repeated model.BranchMember
	f.call("POST", route+"/branches", map[string]any{"name": "implement-01"}, 201, &implementation)
	implementation = f.waitBranchMember(id, implementation.ID, "ready")
	if implementation.Workspace.BaseCommit != group.BaseCommit || implementation.Workspace.Branch != "feat/console/implement-01" {
		t.Fatal("member did not use its group namespace and pinned base")
	}
	f.call("POST", route+"/branches", map[string]any{"name": implementation.Workspace.Branch}, 201, &repeated)
	if repeated.ID != implementation.ID || repeated.Workspace.Path != implementation.Workspace.Path {
		t.Fatal("repeated member creation changed identity")
	}
	if err := os.WriteFile(filepath.Join(implementation.Workspace.Directory, "candidate.txt"), []byte("Implementation candidate"), 0644); err != nil {
		t.Fatal(err)
	}
	gitCommand(t, implementation.Workspace.Directory, "add", "candidate.txt")
	gitCommand(t, implementation.Workspace.Directory, "commit", "-m", "Candidate")
	candidate := gitCommand(t, implementation.Workspace.Directory, "rev-parse", "HEAD")
	f.call("POST", route+"/branches", map[string]any{"name": "optimize-01", "sourceBranchId": implementation.ID}, 201, &optimization)
	optimization = f.waitBranchMember(id, optimization.ID, "ready")
	if optimization.Workspace.BaseCommit != candidate || implementation.Workspace.Path == optimization.Workspace.Path {
		t.Fatal("follow-up member lost its source or isolation")
	}
	f.call("POST", route+"/branches", map[string]any{"name": "bad name"}, 400, nil)
	f.call("POST", route+"/branches", map[string]any{"name": "invalid-source", "sourceBranchId": "missing"}, 400, nil)
	f.call("GET", route+"/materials", nil, 200, &catalog)
	if catalog.Scope.Directory != materialDirectory || len(catalog.Files) != 1 || f.requirementSnapshot(id).Workspace != nil {
		t.Fatalf("adding branches changed the document source: %q -> %q, files=%d", materialDirectory, catalog.Scope.Directory, len(catalog.Files))
	}
	var first, second struct{ ID string }
	f.call("POST", "/sessions", map[string]any{"endpointId": f.endpoint, "requirementId": id, "branchId": "missing"}, 400, nil)
	f.call("POST", "/sessions", map[string]any{"endpointId": f.endpoint, "requirementId": id, "branchId": implementation.ID, "cwd": directory}, 400, nil)
	f.call("POST", "/sessions", map[string]any{"endpointId": f.endpoint, "requirementId": id, "branchId": implementation.ID}, 201, &first)
	f.call("POST", "/sessions", map[string]any{"endpointId": f.endpoint, "requirementId": id, "branchId": optimization.ID}, 201, &second)
	before := f.snapshot(first.ID)
	if before.Session.BranchID != implementation.ID || before.Session.Cwd != implementation.Workspace.Directory || f.snapshot(second.ID).Session.Cwd != optimization.Workspace.Directory {
		t.Fatal("sessions did not receive their explicit branch members")
	}
	work := f.submit(first.ID, "group-development", "Implement the agreed requirement")
	f.call("PUT", route+"/branch-group", map[string]any{"prefix": "feat/console-next"}, 200, nil)
	if f.requirementSnapshot(id).BranchGroup.ID != group.ID || f.snapshot(first.ID).Session.Cwd != before.Session.Cwd {
		t.Fatal("prefix edit changed the group or running session identity")
	}
	f.call("PUT", route+"/branch-group/materials", map[string]any{"branchId": implementation.ID}, 200, nil)
	f.call("GET", route+"/materials", nil, 200, &catalog)
	if catalog.Scope.Directory != implementation.Workspace.Directory || len(catalog.Files) != 0 {
		t.Fatal("explicit material source did not retain branch-specific files")
	}
	f.native.finish(work.Session.ProviderID, work.Turns[0].ProviderID, "completed", "Implementation ready")
	eventually(t, func() bool { return f.snapshot(first.ID).Session.State == "idle" })
	f.call("PUT", route+"/branch-group/materials", map[string]any{"branchId": "missing"}, 400, nil)
	f.call("PUT", route+"/branch-group/materials", map[string]any{"branchId": ""}, 200, nil)
	f.call("GET", route+"/materials/"+document.ID, nil, 200, &document)
	if document.Content != "# Shared acceptance\n" {
		t.Fatal("group operations moved or changed the shared requirement")
	}
	var discussion struct{ SessionID, TurnID string }
	f.call("POST", route+"/conversation/turns", map[string]any{"text": "Review our branches", "clientRequestId": "group-context", "section": "gitBranch"}, 202, &discussion)
	eventually(t, func() bool { return f.snapshot(discussion.SessionID).Turns[0].ProviderID != "" })
	context := f.snapshot(discussion.SessionID).Turns[0].Context
	if !strings.Contains(context, `"prefix":"feat/console-next"`) || !strings.Contains(context, implementation.ID) || !strings.Contains(context, `"section":"Branch group"`) {
		t.Fatal("requirement discussion did not receive group membership")
	}
	d := f.snapshot(discussion.SessionID)
	f.native.finish(d.Session.ProviderID, d.Turns[0].ProviderID, "completed", "Branches aligned")
	eventually(t, func() bool { return f.snapshot(discussion.SessionID).Session.State == "idle" })
	before = f.snapshot(first.ID)
	f.stop()
	f.start()
	f.waitReady()
	eventually(t, func() bool { return f.snapshot(first.ID).Session.State == "idle" })
	after := f.snapshot(first.ID)
	restored := f.requirementSnapshot(id)
	if restored.BranchGroup.ID != group.ID || restored.BranchGroup.Prefix != "feat/console-next" || len(restored.BranchGroup.Members) != 2 || restored.BranchGroup.MaterialBranchID != "" || after.Session.BranchID != before.Session.BranchID || after.Session.ProviderID != before.Session.ProviderID || after.Session.Cwd != before.Session.Cwd || len(after.Entries) != len(before.Entries) {
		t.Fatal("restart lost group membership, source, native identity, or history")
	}
}
