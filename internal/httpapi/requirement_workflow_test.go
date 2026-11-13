package httpapi_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/liyiligang/loom/internal/controller"
	"github.com/liyiligang/loom/internal/model"
	"github.com/liyiligang/loom/internal/requirementflow"
)

func workflowRoute(id string) string { return "/requirements/" + id + "/workflow" }
func readWorkflow(f *fixture, id string) controller.WorkflowDetail {
	var value controller.WorkflowDetail
	f.call("GET", workflowRoute(id), nil, 200, &value)
	return value
}
func waitWorkflow(f *fixture, id, stage, state string) *model.RequirementWorkflow {
	f.t.Helper()
	var w *model.RequirementWorkflow
	eventually(f.t, func() bool {
		w = readWorkflow(f, id).Workflow
		return w != nil && w.Stage == stage && w.State == state
	})
	return w
}
func writeWorkflowFile(t *testing.T, directory, id, relative, content string) {
	t.Helper()
	name := filepath.Join(directory, "docs", "requirements", id, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(name), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}
func stageReceipt(t *testing.T, directory, id string, w *model.RequirementWorkflow, questions []requirementflow.Question, resolved []string) {
	t.Helper()
	receipt := requirementflow.Receipt{SchemaVersion: 1, WorkflowID: w.ID, Stage: w.Stage, RequestID: w.RequestID, AcceptanceIDs: []string{"AC1"}, Questions: questions, ResolvedIDs: resolved}
	raw, _ := json.Marshal(receipt)
	writeWorkflowFile(t, directory, id, ".workflow/"+requirementflow.ReceiptPath(w.ID, w.RequestID), string(raw))
}
func finishWorkflowTurn(f *fixture, w *model.RequirementWorkflow) {
	f.t.Helper()
	var current detail
	eventually(f.t, func() bool {
		current = f.snapshot(w.SessionID)
		for _, turn := range current.Turns {
			if turn.ID == w.TurnID {
				return turn.ProviderID != ""
			}
		}
		return false
	})
	for _, turn := range current.Turns {
		if turn.ID == w.TurnID {
			f.native.finish(current.Session.ProviderID, turn.ProviderID, "completed", "Prepared real document files.")
			return
		}
	}
}
func prepareUntilGrill(f *fixture, id, directory string, questions []requirementflow.Question) *model.RequirementWorkflow {
	f.t.Helper()
	f.call("POST", workflowRoute(id)+"/actions", map[string]any{"action": "start", "brief": "Build a sign-in flow with measurable acceptance criteria.", "clientRequestId": "start-preparation"}, 200, nil)
	w := waitWorkflow(f, id, "requirements", "running")
	writeWorkflowFile(f.t, directory, id, "documents/requirements.md", "# Sign in\n\nAC1: A valid account can sign in.\n")
	stageReceipt(f.t, directory, id, w, nil, nil)
	finishWorkflowTurn(f, w)
	w = waitWorkflow(f, id, "design", "running")
	writeWorkflowFile(f.t, directory, id, "design-documents/design.md", "# Sign-in design\n\nAC1: Verify credentials and create a bounded session.\n")
	stageReceipt(f.t, directory, id, w, nil, nil)
	finishWorkflowTurn(f, w)
	w = waitWorkflow(f, id, "grill", "running")
	writeWorkflowFile(f.t, directory, id, "decisions/grill-me.md", "# Grill me\n\nQ1: Choose the session lifetime and invalidation policy.\n")
	stageReceipt(f.t, directory, id, w, questions, nil)
	finishWorkflowTurn(f, w)
	if len(questions) == 0 {
		return waitWorkflow(f, id, "alignment", "running")
	}
	return waitWorkflow(f, id, "grill", "questions")
}
func testLoop(hashes map[string]string) requirementflow.Plan {
	return requirementflow.Plan{SchemaVersion: 1, Title: "Sign-in delivery Loop", SourceHashes: hashes,
		Nodes: []requirementflow.Node{
			{ID: "start", Title: "Confirmed documents", Kind: "entry", MaxAttempts: 1},
			{ID: "implement", Title: "Implement sign in", Kind: "task", Description: "Implement credential validation and session creation.", AcceptanceIDs: []string{"AC1"}, Outputs: []string{"Sign-in handler"}, Checks: []string{"Build the handler"}, MaxAttempts: 3},
			{ID: "verify", Title: "Verify sign in", Kind: "check", Description: "Validate successful and denied authentication.", AcceptanceIDs: []string{"AC1"}, Outputs: []string{"Authentication test evidence"}, Checks: []string{"Valid credentials pass and invalid credentials fail"}, MaxAttempts: 3},
			{ID: "done", Title: "Accepted sign in", Kind: "exit", MaxAttempts: 1},
		}, Edges: []requirementflow.Edge{{From: "start", To: "implement", Condition: "success"}, {From: "implement", To: "verify", Condition: "success"}, {From: "verify", To: "done", Condition: "success"}, {From: "verify", To: "implement", Condition: "failure", MaxTraversals: 2}}}
}

func TestPreparationWorkflowPersistsHumanGatesAndGeneratesDocumentBackedPreview(t *testing.T) {
	f, id, directory := materialFixture(t)
	w := prepareUntilGrill(f, id, directory, []requirementflow.Question{{ID: "Q1", Question: "How long should sessions last?", Reason: "Expiry affects storage and tests."}})
	f.call("POST", "/requirements/"+id+"/conversation/turns", map[string]any{"text": "Discuss assumptions", "section": "documents", "clientRequestId": "review-discussion"}, 202, nil)
	var discussion detail
	eventually(t, func() bool {
		discussion = f.snapshot(w.SessionID)
		return len(discussion.Turns) == 4 && discussion.Turns[3].ProviderID != ""
	})
	f.native.finish(discussion.Session.ProviderID, discussion.Turns[3].ProviderID, "completed", "Clarified assumptions without changing files.")
	eventually(t, func() bool { return f.snapshot(w.SessionID).Session.State == "idle" })
	f.stop()
	f.start()
	f.waitReady()
	w = waitWorkflow(f, id, "grill", "questions")
	if len(w.Questions) != 1 {
		t.Fatal("restart lost the human gate")
	}
	f.call("POST", workflowRoute(id)+"/actions", map[string]any{"action": "answers", "answers": map[string]string{}, "revision": w.Revision, "clientRequestId": "empty-answers"}, 400, nil)
	answer := map[string]any{"action": "answers", "answers": map[string]string{"Q1": "Expire after 30 minutes."}, "revision": w.Revision, "clientRequestId": "answer-Q1"}
	f.call("POST", workflowRoute(id)+"/actions", answer, 200, nil)
	f.call("POST", workflowRoute(id)+"/actions", answer, 200, nil)
	w = waitWorkflow(f, id, "alignment", "running")
	f.call("POST", "/requirements/"+id+"/conversation/turns", map[string]any{"text": "Competing edit", "section": "documents", "clientRequestId": "competing"}, 409, nil)
	f.call("POST", "/sessions/"+w.SessionID+"/turns", map[string]any{"text": "Competing direct edit", "clientRequestId": "direct-competing"}, 409, nil)
	detail := f.snapshot(w.SessionID)
	if !strings.Contains(detail.Turns[len(detail.Turns)-1].Context, "Expire after 30 minutes.") {
		t.Fatal("alignment did not receive the durable human answer")
	}
	writeWorkflowFile(t, directory, id, "design-documents/design.md", "# Aligned design\n\nAC1: Authenticate and expire sessions after 30 minutes.\n")
	stageReceipt(t, directory, id, w, nil, []string{"Q1"})
	finishWorkflowTurn(f, w)
	w = waitWorkflow(f, id, "alignment", "review")
	review := readWorkflow(f, id)
	if len(review.Documents) != 3 || review.Stale {
		t.Fatal("review did not expose current aligned files")
	}
	writeWorkflowFile(t, directory, id, "documents/requirements.md", "# Revised scope\n\nAC1: Valid credentials produce a 30-minute session.\n")
	confirm := map[string]any{"action": "confirm", "revision": w.Revision, "reviewHash": w.ReviewHash, "clientRequestId": "stale-confirm"}
	f.call("POST", workflowRoute(id)+"/actions", confirm, 409, nil)
	if !readWorkflow(f, id).Stale {
		t.Fatal("changed document was not detected")
	}
	f.call("POST", workflowRoute(id)+"/actions", map[string]any{"action": "revise", "brief": "Include the refined session lifetime.", "revision": w.Revision, "clientRequestId": "realign"}, 200, nil)
	w = waitWorkflow(f, id, "alignment", "running")
	stageReceipt(t, directory, id, w, nil, []string{"Q1"})
	finishWorkflowTurn(f, w)
	w = waitWorkflow(f, id, "alignment", "review")
	confirm = map[string]any{"action": "confirm", "revision": w.Revision, "reviewHash": w.ReviewHash, "clientRequestId": "confirm-current"}
	f.call("POST", workflowRoute(id)+"/actions", confirm, 200, nil)
	f.call("POST", workflowRoute(id)+"/actions", confirm, 200, nil)
	w = waitWorkflow(f, id, "loop", "running")
	if _, err := os.Stat(filepath.Join(directory, "docs/requirements", id, ".workflow", w.ID, "approved", requirementflow.ReviewHash(w.ScopeKey, w.Commit, w.Hashes), "documents/requirements.md")); err != nil {
		t.Fatal("confirmation did not freeze source files", err)
	}
	plan := testLoop(w.Hashes)
	raw, _ := json.Marshal(plan)
	writeWorkflowFile(t, directory, id, "loop-design/loop.json", string(raw))
	writeWorkflowFile(t, directory, id, "loop-design/loop-design.md", "# Sign-in delivery Loop\n\nImplement, verify AC1 and repair at most twice.\n")
	stageReceipt(t, directory, id, w, nil, nil)
	finishWorkflowTurn(f, w)
	w = waitWorkflow(f, id, "loop", "preview")
	preview := readWorkflow(f, id)
	if preview.Plan == nil || len(preview.Plan.Nodes) != 4 || preview.Stale {
		t.Fatal("actual plan was not accepted")
	}
	writeWorkflowFile(t, directory, id, "loop-design/loop.json", "{\"external\":\"change\"}")
	f.stop()
	f.start()
	f.waitReady()
	preview = readWorkflow(f, id)
	if preview.Plan == nil || preview.Plan.Title != plan.Title {
		t.Fatal("external edit rewrote accepted preview provenance")
	}
	f.native.mu.Lock()
	defer f.native.mu.Unlock()
	if f.native.starts != 7 {
		t.Fatalf("expected 7 preparation/discussion turns, got %d", f.native.starts)
	}
}

func TestPreparationStopsWhenFilesAreMissingAndCancellationDoesNotAdvance(t *testing.T) {
	f, id, directory := materialFixture(t)
	start := map[string]any{"action": "start", "brief": "Prepare a sign-in flow.", "clientRequestId": "missing-files"}
	f.call("POST", workflowRoute(id)+"/actions", start, 200, nil)
	w := waitWorkflow(f, id, "requirements", "running")
	finishWorkflowTurn(f, w)
	w = waitWorkflow(f, id, "requirements", "failed")
	if !strings.Contains(w.Error, "receipt") {
		t.Fatal("completion alone advanced the workflow")
	}
	f.call("POST", workflowRoute(id)+"/actions", map[string]any{"action": "retry", "revision": w.Revision, "clientRequestId": "retry-requirements"}, 200, nil)
	w = waitWorkflow(f, id, "requirements", "running")
	writeWorkflowFile(t, directory, id, "documents/requirements.md", "# Sign in\n\nAC1: Valid account signs in.\n")
	stageReceipt(t, directory, id, w, nil, nil)
	f.call("POST", workflowRoute(id)+"/actions", map[string]any{"action": "cancel", "revision": w.Revision, "clientRequestId": "pause"}, 200, nil)
	finishWorkflowTurn(f, w)
	f.stop()
	f.start()
	f.waitReady()
	w = waitWorkflow(f, id, "requirements", "cancelled")
	f.native.mu.Lock()
	starts := f.native.starts
	f.native.mu.Unlock()
	if starts != 2 {
		t.Fatal("cancellation dispatched another stage")
	}
	f.call("POST", workflowRoute(id)+"/actions", map[string]any{"action": "resume", "revision": w.Revision, "clientRequestId": "resume-preparation"}, 200, nil)
	w = waitWorkflow(f, id, "design", "running")
	if w.PausedState != "" {
		t.Fatal("resumption did not restore the original stage state")
	}
	f.native.mu.Lock()
	defer f.native.mu.Unlock()
	if f.native.starts > 3 {
		t.Fatal("resumption resubmitted the completed requirement stage")
	}
}

func TestPreparationRecoversLostAcknowledgementWithoutBlindResubmission(t *testing.T) {
	f, id, _ := materialFixture(t)
	f.native.mu.Lock()
	f.native.loseAck = "identifiable"
	f.native.mu.Unlock()
	f.call("POST", workflowRoute(id)+"/actions", map[string]any{"action": "start", "brief": "Recover this preparation.", "clientRequestId": "lost-preparation"}, 200, nil)
	w := waitWorkflow(f, id, "requirements", "failed")
	if w.TurnID == "" {
		t.Fatal("lost acknowledgement discarded the native turn")
	}
	f.stop()
	f.start()
	f.waitReady()
	w = waitWorkflow(f, id, "requirements", "failed")
	f.native.mu.Lock()
	defer f.native.mu.Unlock()
	if f.native.starts != 1 {
		t.Fatal("recovery blindly resubmitted the preparation turn")
	}
}

func TestPreparationRejectsChangedMaterialSourceAndWrongReceipt(t *testing.T) {
	f, id, directory := materialFixture(t)
	f.call("POST", workflowRoute(id)+"/actions", map[string]any{"action": "start", "brief": "Prepare this feature.", "clientRequestId": "wrong-receipt"}, 200, nil)
	w := waitWorkflow(f, id, "requirements", "running")
	writeWorkflowFile(t, directory, id, "documents/requirements.md", "# Sign in\n\nAC1: Valid account signs in.\n")
	stageReceipt(t, directory, id, w, nil, nil)
	writeWorkflowFile(t, directory, id, ".workflow/"+requirementflow.ReceiptPath(w.ID, w.RequestID), `{"schemaVersion":1,"workflowId":"another","stage":"requirements","requestId":"another","acceptanceIds":["AC1"]}`)
	finishWorkflowTurn(f, w)
	w = waitWorkflow(f, id, "requirements", "failed")
	if !strings.Contains(w.Error, "identify") {
		t.Fatal("wrong attempt receipt was accepted")
	}
	f.call("PATCH", "/requirements/"+id, map[string]any{"branch": "feature/changed-material-source"}, 200, nil)
	f.waitWorkspace(id, "ready")
	f.call("POST", workflowRoute(id)+"/actions", map[string]any{"action": "retry", "revision": w.Revision, "clientRequestId": "retry-stale"}, 200, nil)
	w = waitWorkflow(f, id, "requirements", "failed")
	if !strings.Contains(w.Error, "source") {
		t.Fatal("new source reused old workflow input")
	}
	f.native.mu.Lock()
	defer f.native.mu.Unlock()
	if f.native.starts != 1 {
		t.Fatal("changed source dispatched an old workflow stage")
	}
}
