package requirementflow

import (
	"encoding/json"
	"testing"
)

func validPlan() Plan {
	return Plan{SchemaVersion: 1, Title: "Delivery", SourceHashes: map[string]string{"requirements": "approved"}, Nodes: []Node{
		{ID: "start", Title: "Start", Kind: "entry", MaxAttempts: 1},
		{ID: "build", Title: "Build", Kind: "task", Description: "Build the feature.", Outputs: []string{"Implementation"}, Checks: []string{"Build succeeds"}, MaxAttempts: 3},
		{ID: "test", Title: "Test", Kind: "check", Description: "Check the acceptance criteria.", AcceptanceIDs: []string{"AC1"}, Outputs: []string{"Evidence"}, Checks: []string{"AC1 passes"}, MaxAttempts: 3},
		{ID: "done", Title: "Done", Kind: "exit", MaxAttempts: 1},
	}, Edges: []Edge{{From: "start", To: "build", Condition: "success"}, {From: "build", To: "test", Condition: "success"}, {From: "test", To: "done", Condition: "success"}, {From: "test", To: "build", Condition: "failure", MaxTraversals: 2}}}
}
func TestPlanRejectsUnboundedFeedbackDisconnectedNodesAndUncoveredAcceptance(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*Plan)
	}{
		{"valid", func(*Plan) {}},
		{"unbounded feedback", func(p *Plan) { p.Edges[3].MaxTraversals = 0 }},
		{"success cycle", func(p *Plan) { p.Edges = append(p.Edges, Edge{From: "test", To: "build", Condition: "success"}) }},
		{"disconnected", func(p *Plan) { p.Edges = p.Edges[1:] }},
		{"uncovered acceptance", func(p *Plan) { p.Nodes[2].AcceptanceIDs = nil }},
		{"source mismatch", func(p *Plan) { p.SourceHashes = map[string]string{"requirements": "stale"} }},
		{"unknown kind", func(p *Plan) { p.Nodes[1].Kind = "script" }},
		{"dangling edge", func(p *Plan) { p.Edges[0].To = "missing" }},
		{"duplicate node", func(p *Plan) { p.Nodes[1].ID = "start" }},
		{"missing check", func(p *Plan) { p.Nodes[2].Checks = nil }},
		{"enter entry", func(p *Plan) { p.Edges[3].To = "start" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			p := validPlan()
			test.mutate(&p)
			raw, _ := json.Marshal(p)
			_, err := ParsePlan(raw, map[string]string{"requirements": "approved"}, []string{"AC1"})
			if (err == nil) != (test.name == "valid") {
				t.Fatalf("unexpected plan validation: %v", err)
			}
		})
	}
}
