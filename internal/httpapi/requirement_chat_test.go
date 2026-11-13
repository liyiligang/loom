package httpapi_test

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/liyiligang/loom/internal/model"
	"github.com/liyiligang/loom/internal/storage"
)

func requirementForChat(t *testing.T, f *fixture) string {
	t.Helper()
	var project, requirement struct{ ID string }
	f.call("POST", "/projects", map[string]any{"endpointId": f.endpoint, "cwd": "/target/project", "name": "Planning project"}, 201, &project)
	f.call("POST", "/requirements", map[string]any{"projectId": project.ID, "name": "Account design"}, 201, &requirement)
	return requirement.ID
}

func TestRequirementConversationPersistsAndUsesCurrentSection(t *testing.T) {
	f := setup(t)
	id := requirementForChat(t, f)
	var feature struct{ ID string }
	f.call("POST", "/sessions", map[string]any{"endpointId": f.endpoint, "requirementId": id, "title": "Implementation"}, 201, &feature)
	var first struct{ SessionID, TurnID string }
	message := map[string]any{"text": "Align the sign-in design", "clientRequestId": "discussion-1", "section": "designDocuments"}
	f.call("POST", "/requirements/"+id+"/conversation/turns", message, 202, &first)
	var discussion detail
	eventually(t, func() bool {
		f.call("GET", "/sessions/"+first.SessionID, nil, 200, &discussion)
		return discussion.Turns[0].ProviderID != ""
	})
	if discussion.Session.Purpose != "requirement" || first.SessionID == feature.ID || discussion.Session.Cwd != "/target/project" || discussion.Session.RequirementID != id {
		t.Fatal("requirement conversation did not receive its own persistent identity and project scope")
	}
	f.native.mu.Lock()
	native := f.native.threads[discussion.Session.ProviderID].Turns[0].Items[0]["content"].([]map[string]any)[0]["text"].(string)
	f.native.mu.Unlock()
	if !strings.Contains(native, `"section":"Design documents"`) || !strings.Contains(native, `"requirementName":"Account design"`) || !strings.HasSuffix(native, "Align the sign-in design") {
		t.Fatalf("missing requirement context: %s", native)
	}
	f.native.finish(discussion.Session.ProviderID, discussion.Turns[0].ProviderID, "completed", "The documents now share the same assumptions.")
	eventually(t, func() bool {
		f.call("GET", "/sessions/"+first.SessionID, nil, 200, &discussion)
		return discussion.Session.State == "idle"
	})
	if discussion.Turns[0].Input != "Align the sign-in design" || discussion.Entries[0].Text != "Align the sign-in design" {
		t.Fatal("native context leaked into the displayed user message or changed its deduplication identity")
	}
	var duplicate struct{ SessionID, TurnID string }
	f.call("POST", "/requirements/"+id+"/conversation/turns", message, 202, &duplicate)
	if duplicate != first {
		t.Fatal("retry created another conversation or turn")
	}
	f.stop()
	f.start()
	f.waitReady()
	eventually(t, func() bool { return f.snapshot(first.SessionID).Session.State == "idle" })
	var second struct{ SessionID, TurnID string }
	f.call("POST", "/requirements/"+id+"/conversation/turns", map[string]any{"text": "Check the test cases against those decisions", "clientRequestId": "discussion-2", "section": "testCases"}, 202, &second)
	if second.SessionID != first.SessionID || second.TurnID == first.TurnID {
		t.Fatal("section change or restart replaced the conversation")
	}
	f.call("GET", "/sessions/"+first.SessionID, nil, 200, &discussion)
	if discussion.Session.Purpose != "requirement" || len(discussion.Turns) != 2 || !strings.Contains(discussion.Turns[1].Context, `"section":"Test cases"`) {
		t.Fatal("history, purpose, or current section did not survive restart")
	}
	var other struct{ ID string }
	var groups navigation
	f.call("GET", "/bootstrap", nil, 200, &groups)
	f.call("POST", "/requirements", map[string]any{"projectId": groups.Projects[0].ID, "name": "Other requirement"}, 201, &other)
	f.call("PATCH", "/sessions/"+first.SessionID, map[string]any{"requirementId": other.ID}, 400, nil)
}

func TestRequirementConversationConcurrentRetriesCreateOneNativeThread(t *testing.T) {
	f := setup(t)
	id := requirementForChat(t, f)
	var wg sync.WaitGroup
	results := make([]struct{ SessionID, TurnID string }, 6)
	for index := range results {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			f.call("POST", "/requirements/"+id+"/conversation/turns", map[string]any{"text": "Agree on the requirements", "clientRequestId": "same-request", "section": "documents"}, 202, &results[index])
		}(index)
	}
	wg.Wait()
	for _, result := range results {
		if result != results[0] {
			t.Fatal("concurrent retries did not share one operation")
		}
	}
	f.native.mu.Lock()
	defer f.native.mu.Unlock()
	if len(f.native.threads) != 1 {
		t.Fatal("concurrent sends created duplicate native conversations")
	}
}

func TestRequirementConversationValidatesBeforeNativeCreation(t *testing.T) {
	f := setup(t)
	id := requirementForChat(t, f)
	for index, input := range []map[string]any{
		{"text": "", "clientRequestId": "empty", "section": "documents"},
		{"text": "Discuss", "clientRequestId": "section", "section": "unknown"},
		{"text": "Discuss", "section": "documents"},
	} {
		t.Run(fmt.Sprint(index), func(t *testing.T) { f.call("POST", "/requirements/"+id+"/conversation/turns", input, 400, nil) })
	}
	f.call("POST", "/requirements/missing/conversation/turns", map[string]any{"text": "Discuss", "clientRequestId": "missing", "section": "documents"}, 404, nil)
	var snapshot struct{ Sessions []model.Session }
	f.call("GET", "/bootstrap", nil, 200, &snapshot)
	f.native.mu.Lock()
	defer f.native.mu.Unlock()
	if len(snapshot.Sessions) != 0 || len(f.native.threads) != 0 {
		t.Fatal("invalid input created a conversation")
	}
}

func TestRequirementConversationReconcilesContextAfterLostAcknowledgement(t *testing.T) {
	f := setup(t)
	id := requirementForChat(t, f)
	f.native.mu.Lock()
	f.native.loseAck = "single"
	f.native.mu.Unlock()
	input := map[string]any{"text": "Review the Loop Design", "clientRequestId": "lost-discussion", "section": "loopDesign"}
	var sent struct{ SessionID, TurnID string }
	f.call("POST", "/requirements/"+id+"/conversation/turns", input, 202, &sent)
	var current detail
	eventually(t, func() bool {
		f.call("GET", "/sessions/"+sent.SessionID, nil, 200, &current)
		return current.Session.State == "syncing"
	})
	f.waitReady()
	eventually(t, func() bool { return f.snapshot(sent.SessionID).Session.State == "idle" })
	f.call("GET", "/sessions/"+sent.SessionID, nil, 200, &current)
	if current.Session.State != "idle" || len(current.Turns) != 1 || current.Turns[0].ID != sent.TurnID || current.Turns[0].Input != input["text"] || current.Entries[0].Text != input["text"] {
		t.Fatal("context prevented safe acknowledgement recovery or changed the user message")
	}
	var duplicate struct{ SessionID, TurnID string }
	f.call("POST", "/requirements/"+id+"/conversation/turns", input, 202, &duplicate)
	if duplicate != sent {
		t.Fatal("retry after acknowledgement recovery duplicated the discussion")
	}
	f.native.mu.Lock()
	defer f.native.mu.Unlock()
	if f.native.starts != 1 {
		t.Fatal("acknowledgement recovery resubmitted a model turn")
	}
}

func TestRequirementConversationRecoversBeforeItsFirstAcceptedMessage(t *testing.T) {
	f := setup(t)
	id := requirementForChat(t, f)
	var created struct{ ID string }
	f.call("POST", "/sessions", map[string]any{"endpointId": f.endpoint, "requirementId": id}, 201, &created)
	before := f.snapshot(created.ID)
	f.stop()
	// Emulate shutdown after durable conversation creation but before Submit.
	store, err := storage.Open(filepath.Join(f.dataPath, "loom.db"))
	if err != nil {
		t.Fatal(err)
	}
	before.Session.Purpose = "requirement"
	_, err = store.Commit("session.updated", created.ID, before.Session)
	store.Close()
	if err != nil {
		t.Fatal(err)
	}
	f.start()
	f.waitReady()
	eventually(t, func() bool {
		current := f.snapshot(created.ID)
		return current.Session.State == "idle" && current.Session.ProviderID != before.Session.ProviderID
	})
	var result struct{ SessionID, TurnID string }
	f.call("POST", "/requirements/"+id+"/conversation/turns", map[string]any{"text": "Start the discussion", "clientRequestId": "first-after-recovery", "section": "documents"}, 202, &result)
	if result.SessionID != created.ID || len(f.snapshot(created.ID).Turns) != 1 {
		t.Fatal("recovering an empty native thread replaced the requirement conversation or lost its first message")
	}
}

func TestRequirementConversationRetainsProjectScopeAcrossBranchChanges(t *testing.T) {
	f, _ := localFixture(t, true, nil)
	_ = f.localReady()
	f.call("POST", "/local/confirm", nil, 200, nil)
	f.waitReady()
	directory := gitProject(t)
	directory, err := filepath.EvalSymlinks(directory)
	if err != nil {
		t.Fatal(err)
	}
	var project, requirement struct{ ID string }
	f.call("POST", "/projects", map[string]any{"endpointId": "local-codex", "cwd": directory}, 201, &project)
	f.call("POST", "/requirements", map[string]any{"projectId": project.ID, "name": "Branch planning", "branch": "feature/planning"}, 201, &requirement)
	f.waitWorkspace(requirement.ID, "ready")
	var result struct{ SessionID, TurnID string }
	f.call("POST", "/requirements/"+requirement.ID+"/conversation/turns", map[string]any{"text": "Align the design", "clientRequestId": "branch-discussion", "section": "designDocuments"}, 202, &result)
	eventually(t, func() bool { return f.snapshot(result.SessionID).Turns[0].ProviderID != "" })
	before := f.snapshot(result.SessionID)
	if before.Session.Cwd != directory || before.Session.Workspace != nil || !strings.Contains(before.Turns[0].Context, `"branch":"feature/planning"`) {
		t.Fatal("requirement discussion inherited the execution worktree or lost its branch context")
	}
	f.native.finish(before.Session.ProviderID, before.Turns[0].ProviderID, "completed", "Recorded the design")
	eventually(t, func() bool { return f.snapshot(result.SessionID).Session.State == "idle" })
	f.call("PATCH", "/requirements/"+requirement.ID, map[string]any{"branch": "feature/revised"}, 200, nil)
	f.waitWorkspace(requirement.ID, "ready")
	f.stop()
	f.start()
	f.waitReady()
	eventually(t, func() bool { return f.snapshot(result.SessionID).Session.State == "idle" })
	var next struct{ SessionID, TurnID string }
	f.call("POST", "/requirements/"+requirement.ID+"/conversation/turns", map[string]any{"text": "Compare the revised tests", "clientRequestId": "revised-discussion", "section": "testCases"}, 202, &next)
	after := f.snapshot(result.SessionID)
	if next.SessionID != result.SessionID || after.Session.ProviderID != before.Session.ProviderID || after.Session.Cwd != directory || after.Session.Workspace != nil || !strings.Contains(after.Turns[1].Context, `"branch":"feature/revised"`) {
		t.Fatal("branch replacement or restart repointed the conversation or lost current branch context")
	}
}
