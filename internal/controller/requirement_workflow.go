package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"reflect"
	"strings"
	"time"

	"github.com/liyiligang/loom/internal/materials"
	"github.com/liyiligang/loom/internal/model"
	"github.com/liyiligang/loom/internal/requirementflow"
)

type WorkflowAction struct {
	Action          string            `json:"action"`
	ClientRequestID string            `json:"clientRequestId"`
	Revision        int               `json:"revision"`
	Brief           string            `json:"brief"`
	EndpointID      string            `json:"endpointId"`
	Answers         map[string]string `json:"answers"`
	ReviewHash      string            `json:"reviewHash"`
}
type WorkflowDetail struct {
	Workflow  *model.RequirementWorkflow `json:"workflow"`
	Documents []requirementflow.Document `json:"documents"`
	Plan      *requirementflow.Plan      `json:"plan"`
	Stale     bool                       `json:"stale"`
	Warning   string                     `json:"warning,omitempty"`
}

func (s *Service) lockWorkflow(ctx context.Context, id string) (func(), error) {
	value, _ := s.workflowLocks.LoadOrStore(id, make(chan struct{}, 1))
	gate := value.(chan struct{})
	select {
	case gate <- struct{}{}:
		return func() { <-gate }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
func (s *Service) workflow(id string) *model.RequirementWorkflow {
	for _, w := range s.data.Workflows {
		if w.RequirementID == id {
			return w
		}
	}
	return nil
}
func (s *Service) saveWorkflow(w *model.RequirementWorkflow) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	w.Revision++
	w.UpdatedAt = model.Now()
	existing := s.workflow(w.RequirementID)
	if existing == nil {
		existing = w
		s.data.Workflows = append(s.data.Workflows, existing)
	} else {
		*existing = *w
	}
	return s.commitLocked("workflow.updated", w.RequirementID, existing)
}
func (s *Service) copyWorkflow(id string) (*model.RequirementWorkflow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.requirement(id) == nil {
		return nil, problem("requirement_not_found", "Requirement not found.", 404)
	}
	w := s.workflow(id)
	if w == nil {
		return nil, nil
	}
	raw, _ := json.Marshal(w)
	var copy model.RequirementWorkflow
	_ = json.Unmarshal(raw, &copy)
	return &copy, nil
}

func (s *Service) RequirementWorkflow(ctx context.Context, id string) (WorkflowDetail, error) {
	release, err := s.lockWorkflow(ctx, id)
	if err != nil {
		return WorkflowDetail{}, err
	}
	defer release()
	return s.inspectWorkflow(ctx, id)
}
func (s *Service) inspectWorkflow(ctx context.Context, id string) (WorkflowDetail, error) {
	w, err := s.copyWorkflow(id)
	result := WorkflowDetail{Workflow: w, Documents: []requirementflow.Document{}}
	if err != nil || w == nil {
		return result, err
	}
	scope, release, err := s.materialScope(ctx, id, false)
	if err != nil {
		result.Stale = true
		result.Warning = err.Error()
		return result, nil
	}
	defer release()
	result.Stale = scope.Key != w.ScopeKey || scope.Commit != w.Commit
	if len(w.Hashes) > 0 {
		documents, hashes, err := requirementflow.ReadDocuments(ctx, scope, len(w.Hashes))
		if err != nil {
			result.Stale = true
			result.Warning = err.Error()
		} else {
			result.Stale = result.Stale || !reflect.DeepEqual(hashes, w.Hashes)
			if w.State == "review" {
				result.Documents = documents
			}
		}
	}
	if w.State == "preview" && scope.Key == w.ScopeKey {
		raw, err := materials.ReadWorkflow(scope, path.Join(w.ID, "previews", w.RequestID+".json"))
		if err == nil {
			result.Plan, err = requirementflow.ParsePlan(raw, w.Hashes, w.Criteria)
		}
		if err != nil {
			result.Warning = err.Error()
		}
	}
	return result, nil
}
func (s *Service) ApplyWorkflowAction(ctx context.Context, id string, input WorkflowAction) (WorkflowDetail, error) {
	if input.ClientRequestID == "" || len(input.ClientRequestID) > 128 || len(input.Brief) > 32768 {
		return WorkflowDetail{}, problem("invalid_workflow", "Provide a request ID and a brief of at most 32 KiB.", 400)
	}
	release, err := s.lockWorkflow(ctx, id)
	if err != nil {
		return WorkflowDetail{}, err
	}
	defer release()
	w, err := s.copyWorkflow(id)
	if err != nil {
		return WorkflowDetail{}, err
	}
	if w != nil && w.LastActionID == input.ClientRequestID {
		return s.inspectWorkflow(ctx, id)
	}
	if w != nil && input.Revision != w.Revision {
		return WorkflowDetail{}, problem("workflow_conflict", "The workflow changed. Refresh before continuing.", 409)
	}
	if input.Action == "start" {
		if w != nil && (w.State == "pending" || w.State == "running") {
			return WorkflowDetail{}, problem("workflow_busy", "Preparation is already running.", 409)
		}
		if strings.TrimSpace(input.Brief) == "" {
			return WorkflowDetail{}, problem("invalid_workflow", "Describe the requirement before starting preparation.", 400)
		}
		scope, releaseScope, err := s.materialScope(ctx, id, true)
		if err != nil {
			return WorkflowDetail{}, err
		}
		releaseScope()
		revision := 0
		if w != nil {
			revision = w.Revision
		}
		w = &model.RequirementWorkflow{ID: model.ID(), RequirementID: id, Revision: revision, State: "pending", Stage: "requirements", Attempt: 1, Brief: input.Brief, EndpointID: input.EndpointID, ScopeKey: scope.Key, Commit: scope.Commit, Hashes: map[string]string{}, Answers: map[string]string{}, Criteria: []string{}, Questions: []requirementflow.Question{}}
		w.RequestID = model.ID()
	} else {
		if w == nil {
			return WorkflowDetail{}, problem("workflow_not_found", "Start requirement preparation first.", 409)
		}
		switch input.Action {
		case "answers":
			if w.State != "questions" {
				return WorkflowDetail{}, problem("workflow_conflict", "This workflow is not waiting for Grill me answers.", 409)
			}
			if len(input.Answers) != len(w.Questions) {
				return WorkflowDetail{}, problem("invalid_workflow", "Answer every Grill me question.", 400)
			}
			for _, q := range w.Questions {
				if strings.TrimSpace(input.Answers[q.ID]) == "" || len(input.Answers[q.ID]) > 4000 {
					return WorkflowDetail{}, problem("invalid_workflow", "Answer each question with at most 4000 characters.", 400)
				}
			}
			w.Answers = input.Answers
			queueWorkflow(w, "alignment")
		case "confirm":
			if w.State != "review" || input.ReviewHash != w.ReviewHash {
				return WorkflowDetail{}, problem("workflow_conflict", "Review the aligned documents before confirming them.", 409)
			}
			scope, releaseScope, err := s.materialScope(ctx, id, true)
			if err != nil {
				return WorkflowDetail{}, err
			}
			documents, hashes, err := requirementflow.ReadDocuments(ctx, scope, 3)
			if err == nil && (scope.Key != w.ScopeKey || scope.Commit != w.Commit || !reflect.DeepEqual(hashes, w.Hashes)) {
				err = errors.New("Documents or the Git revision changed. Request another alignment before confirming.")
			}
			if err == nil {
				for _, document := range documents {
					if err = materials.WriteWorkflow(scope, path.Join(w.ID, "approved", w.ReviewHash, document.Path), []byte(document.Content)); err != nil {
						break
					}
				}
			}
			releaseScope()
			if err != nil {
				return WorkflowDetail{}, problem("workflow_stale", err.Error(), 409)
			}
			queueWorkflow(w, "loop")
		case "revise":
			if w.State != "review" && w.State != "preview" && w.State != "failed" {
				return WorkflowDetail{}, problem("workflow_conflict", "Wait for preparation to finish before requesting revisions.", 409)
			}
			if strings.TrimSpace(input.Brief) == "" {
				return WorkflowDetail{}, problem("invalid_workflow", "Describe the requested revisions.", 400)
			}
			w.Brief += "\nRevision request:\n" + input.Brief
			if len(w.Brief) > 32768 {
				return WorkflowDetail{}, problem("invalid_workflow", "The accumulated brief exceeds 32 KiB. Start a new preparation.", 400)
			}
			queueWorkflow(w, "alignment")
		case "retry":
			if w.State != "failed" {
				return WorkflowDetail{}, problem("workflow_conflict", "Only a failed preparation stage can be retried. Synchronize uncertain native turns first.", 409)
			}
			queueWorkflow(w, w.Stage)
		case "cancel":
			if w.State != "cancelled" {
				w.PausedState = w.State
			}
			w.State, w.Error = "cancelled", ""
		case "resume":
			if w.State != "cancelled" || w.PausedState == "" {
				return WorkflowDetail{}, problem("workflow_conflict", "Only a paused preparation can resume.", 409)
			}
			w.State, w.PausedState = w.PausedState, ""
		default:
			return WorkflowDetail{}, problem("invalid_workflow", "Unknown preparation action.", 400)
		}
	}
	w.LastActionID = input.ClientRequestID
	if err := s.saveWorkflow(w); err != nil {
		return WorkflowDetail{}, err
	}
	return s.inspectWorkflow(ctx, id)
}

func queueWorkflow(w *model.RequirementWorkflow, stage string) {
	w.Stage, w.State, w.TurnID, w.Error, w.ReviewHash = stage, "pending", "", "", ""
	w.Attempt++
	w.RequestID = model.ID()
}

func (s *Service) runRequirementWorkflows() {
	defer s.workers.Done()
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.mu.Lock()
			ids := []string{}
			for _, w := range s.data.Workflows {
				if w.State == "pending" || w.State == "running" {
					ids = append(ids, w.RequirementID)
				}
			}
			s.mu.Unlock()
			for _, id := range ids {
				s.advanceWorkflow(id)
			}
		}
	}
}

func (s *Service) advanceWorkflow(id string) {
	release, err := s.lockWorkflow(s.ctx, id)
	if err != nil {
		return
	}
	defer release()
	w, _ := s.copyWorkflow(id)
	if w == nil || (w.State != "pending" && w.State != "running") {
		return
	}
	// A crash can occur after Submit committed but before the workflow attached its turn.
	// Reconcile by the persisted request ID before considering any dispatch.
	s.mu.Lock()
	var turn *model.Turn
	for _, candidate := range s.data.Turns {
		if candidate.ClientRequestID == w.RequestID {
			session := s.session(candidate.SessionID)
			if session != nil && session.RequirementID == id && session.Purpose == "requirement" {
				copy := *candidate
				turn = &copy
				break
			}
		}
	}
	s.mu.Unlock()
	if turn != nil && w.TurnID == "" {
		w.TurnID, w.SessionID, w.State = turn.ID, turn.SessionID, "running"
		s.replaceWorkflow(w)
		return
	}
	if turn == nil && w.State == "pending" {
		scope, releaseScope, err := s.materialScope(s.ctx, id, false)
		if err != nil {
			s.failWorkflow(w, err)
			return
		}
		if scope.Key != w.ScopeKey || scope.Commit != w.Commit {
			releaseScope()
			s.failWorkflow(w, errors.New("The material source or Git revision changed. Restart preparation."))
			return
		}
		// Alignment may revise documents; all other stages must read unchanged inputs.
		if w.Stage != "requirements" && w.Stage != "alignment" && len(w.Hashes) > 0 {
			_, hashes, readErr := requirementflow.ReadDocuments(s.ctx, scope, len(w.Hashes))
			if readErr != nil || !reflect.DeepEqual(hashes, w.Hashes) {
				releaseScope()
				s.failWorkflow(w, errors.New("Upstream documents changed. Request another alignment before proceeding."))
				return
			}
		}
		instructions := workflowInstructions(w, scope)
		releaseScope()
		result, err := s.sendRequirementMessage(s.ctx, id, RequirementMessage{Text: stageMessage(w.Stage), ClientRequestID: w.RequestID, EndpointID: w.EndpointID, Section: stageSection(w.Stage)}, instructions, w.ScopeKey, w.Commit)
		if err != nil {
			s.failWorkflow(w, err)
			return
		}
		w.TurnID, w.SessionID, w.State = result.TurnID, result.SessionID, "running"
		s.replaceWorkflow(w)
		return
	}
	if turn == nil {
		s.failWorkflow(w, errors.New("The preparation turn is missing from its durable ledger."))
		return
	}
	if !terminal(turn.State) {
		return
	}
	if turn.State != "completed" {
		s.failWorkflow(w, errors.New("The native AI turn did not complete. Review the conversation and retry this stage."))
		return
	}
	scope, releaseScope, err := s.materialScope(s.ctx, id, true)
	if err != nil {
		s.failWorkflow(w, err)
		return
	}
	defer releaseScope()
	if scope.Key != w.ScopeKey || scope.Commit != w.Commit {
		s.failWorkflow(w, errors.New("The material source or Git revision changed while AI was preparing documents. Restart preparation."))
		return
	}
	if err := s.acceptWorkflowStage(w, scope); err != nil {
		s.failWorkflow(w, err)
		return
	}
	_, _ = s.scanMaterials(s.ctx, scope)
	s.replaceWorkflow(w)
}

func (s *Service) replaceWorkflow(w *model.RequirementWorkflow) {
	_ = s.saveWorkflow(w)
}
func (s *Service) failWorkflow(w *model.RequirementWorkflow, err error) {
	if s.ctx.Err() != nil {
		return
	}
	w.State, w.Error = "failed", err.Error()
	s.replaceWorkflow(w)
}

func (s *Service) acceptWorkflowStage(w *model.RequirementWorkflow, scope materials.Scope) error {
	receipt, err := requirementflow.ReadReceipt(scope, w.ID, w.Stage, w.RequestID)
	if err != nil {
		return err
	}
	count := 3
	if w.Stage == "requirements" {
		count = 1
	}
	if w.Stage == "design" {
		count = 2
	}
	documents, hashes, err := requirementflow.ReadDocuments(s.ctx, scope, count)
	if err != nil {
		return err
	}
	if w.Stage != "requirements" && w.Stage != "alignment" {
		for name, hash := range w.Hashes {
			if hashes[name] != hash {
				return errors.New("AI changed an upstream document outside the alignment stage. Request another alignment.")
			}
		}
	}
	if w.Stage == "requirements" {
		if err := requirementflow.ValidateIDs(receipt.AcceptanceIDs, 1, 100); err != nil {
			return err
		}
		w.Criteria = receipt.AcceptanceIDs
	} else if !requirementflow.Covers(receipt.AcceptanceIDs, w.Criteria) {
		return errors.New("The stage receipt does not cover every acceptance criterion.")
	}
	for _, document := range documents {
		if document.Path == "decisions/grill-me.md" {
			continue
		}
		for _, id := range w.Criteria {
			if !strings.Contains(document.Content, id) {
				return fmt.Errorf("Document %s does not reference acceptance criterion %s.", document.Path, id)
			}
		}
	}
	switch w.Stage {
	case "requirements":
		w.Hashes = hashes
		queueWorkflow(w, "design")
	case "design":
		w.Hashes = hashes
		queueWorkflow(w, "grill")
	case "grill":
		if err := requirementflow.ValidateQuestions(receipt.Questions); err != nil {
			return err
		}
		w.Hashes, w.Questions, w.State = hashes, receipt.Questions, "questions"
		if len(w.Questions) == 0 {
			queueWorkflow(w, "alignment")
		}
	case "alignment":
		ids := []string{}
		for _, question := range w.Questions {
			ids = append(ids, question.ID)
		}
		if !requirementflow.Covers(receipt.ResolvedIDs, ids) {
			return errors.New("Alignment must resolve every answered Grill me question.")
		}
		w.Hashes, w.State = hashes, "review"
		w.ReviewHash = requirementflow.ReviewHash(w.ScopeKey, w.Commit, hashes)
	case "loop":
		catalog, err := materials.Scan(s.ctx, scope)
		if err != nil {
			return err
		}
		file, err := materials.Find(catalog, materials.FileID(scope, path.Join(scope.Root, "loop-design/loop.json")))
		if err != nil {
			return errors.New("AI did not produce loop-design/loop.json.")
		}
		detail, err := materials.Read(scope, file)
		if err != nil {
			return err
		}
		if detail.Truncated {
			return errors.New("Loop JSON exceeds 1 MiB.")
		}
		if _, err := requirementflow.ParsePlan([]byte(detail.Content), w.Hashes, w.Criteria); err != nil {
			return err
		}
		explanationFile, err := materials.Find(catalog, materials.FileID(scope, path.Join(scope.Root, "loop-design/loop-design.md")))
		if err != nil {
			return errors.New("AI did not produce loop-design/loop-design.md.")
		}
		explanation, err := materials.Read(scope, explanationFile)
		if err != nil || explanation.Truncated || len(strings.TrimSpace(explanation.Content)) < 20 {
			return errors.New("The Loop explanation must be readable substantive Markdown.")
		}
		if err := materials.WriteWorkflow(scope, path.Join(w.ID, "previews", w.RequestID+".json"), []byte(detail.Content)); err != nil {
			return err
		}
		w.State = "preview"
	default:
		return fmt.Errorf("Unknown preparation stage %s.", w.Stage)
	}
	return nil
}
