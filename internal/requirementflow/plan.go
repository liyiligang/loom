package requirementflow

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
)

type Node struct {
	ID            string   `json:"id"`
	Title         string   `json:"title"`
	Kind          string   `json:"kind"`
	Description   string   `json:"description"`
	AcceptanceIDs []string `json:"acceptanceIds"`
	Outputs       []string `json:"outputs"`
	Checks        []string `json:"checks"`
	MaxAttempts   int      `json:"maxAttempts"`
}
type Edge struct {
	From          string `json:"from"`
	To            string `json:"to"`
	Condition     string `json:"condition"`
	MaxTraversals int    `json:"maxTraversals,omitempty"`
}
type Plan struct {
	SchemaVersion int               `json:"schemaVersion"`
	Title         string            `json:"title"`
	SourceHashes  map[string]string `json:"sourceHashes"`
	Nodes         []Node            `json:"nodes"`
	Edges         []Edge            `json:"edges"`
}

func ParsePlan(raw []byte, hashes map[string]string, criteria []string) (*Plan, error) {
	var p Plan
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, errors.New("Loop plan must be valid JSON.")
	}
	if p.SchemaVersion != 1 || strings.TrimSpace(p.Title) == "" || len(p.Title) > 200 || !reflect.DeepEqual(p.SourceHashes, hashes) || len(p.Nodes) < 3 || len(p.Nodes) > 40 || len(p.Edges) < 2 || len(p.Edges) > 100 {
		return nil, errors.New("Loop plan has an invalid schema, source version, title, or graph size.")
	}
	nodes := map[string]Node{}
	entry, exit := "", ""
	covered := []string{}
	allowedCriteria := map[string]bool{}
	for _, id := range criteria {
		allowedCriteria[id] = true
	}
	for _, n := range p.Nodes {
		if !stableID.MatchString(n.ID) || nodes[n.ID].ID != "" || strings.TrimSpace(n.Title) == "" || len(n.Title) > 200 || len(n.Description) > 4000 || n.MaxAttempts < 1 || n.MaxAttempts > 5 {
			return nil, errors.New("Loop nodes need unique IDs, bounded text and 1 to 5 attempts.")
		}
		switch n.Kind {
		case "entry":
			if entry != "" {
				return nil, errors.New("Loop needs one entry.")
			}
			entry = n.ID
		case "exit":
			if exit != "" {
				return nil, errors.New("Loop needs one exit.")
			}
			exit = n.ID
		case "task", "check":
			if len(n.Outputs) == 0 || len(n.Checks) == 0 || strings.TrimSpace(n.Description) == "" {
				return nil, errors.New("Tasks and checks need descriptions, outputs and verification checks.")
			}
		default:
			return nil, errors.New("Unknown Loop node kind.")
		}
		for _, list := range [][]string{n.Outputs, n.Checks} {
			if len(list) > 20 {
				return nil, errors.New("Too many node outputs or checks.")
			}
			for _, text := range list {
				if strings.TrimSpace(text) == "" || len(text) > 2000 {
					return nil, errors.New("Node outputs and checks must be bounded text.")
				}
			}
		}
		if ValidateIDs(n.AcceptanceIDs, 0, 100) != nil {
			return nil, errors.New("Invalid node acceptance criteria.")
		}
		for _, id := range n.AcceptanceIDs {
			if !allowedCriteria[id] {
				return nil, errors.New("Node references an unknown acceptance criterion.")
			}
		}
		if n.Kind == "check" {
			covered = append(covered, n.AcceptanceIDs...)
		}
		nodes[n.ID] = n
	}
	coveredSet := map[string]bool{}
	for _, id := range covered {
		coveredSet[id] = true
	}
	for _, id := range criteria {
		if !coveredSet[id] {
			return nil, fmt.Errorf("No verification node covers acceptance criterion %s.", id)
		}
	}
	if entry == "" || exit == "" {
		return nil, errors.New("Loop needs one entry and one exit.")
	}
	forward, reverse := map[string][]string{}, map[string][]string{}
	seen := map[string]bool{}
	for _, e := range p.Edges {
		key := e.From + "/" + e.To + "/" + e.Condition
		if nodes[e.From].ID == "" || nodes[e.To].ID == "" || seen[key] || e.From == exit || e.To == entry {
			return nil, errors.New("Loop edges need valid distinct references and valid entry/exit routing.")
		}
		seen[key] = true
		if e.Condition == "failure" {
			if e.MaxTraversals < 1 || e.MaxTraversals > 5 || nodes[e.From].Kind != "check" || nodes[e.To].Kind != "task" {
				return nil, errors.New("Failure routes must go from a check to a task with a finite retry budget of 1 to 5.")
			}
		} else if e.Condition == "success" {
			if e.MaxTraversals != 0 {
				return nil, errors.New("Success routes do not have retry budgets.")
			}
			forward[e.From] = append(forward[e.From], e.To)
			reverse[e.To] = append(reverse[e.To], e.From)
		} else {
			return nil, errors.New("Unknown Loop edge condition.")
		}
	}
	for index, graph := range []map[string][]string{forward, reverse} {
		start := entry
		if index == 1 {
			start = exit
		}
		visited := map[string]bool{}
		var walk func(string)
		walk = func(id string) {
			if visited[id] {
				return
			}
			visited[id] = true
			for _, to := range graph[id] {
				walk(to)
			}
		}
		walk(start)
		if len(visited) != len(nodes) {
			return nil, errors.New("Every node must be reachable from entry and able to reach exit through success routes.")
		}
	}
	colors := map[string]int{}
	var acyclic func(string) bool
	acyclic = func(id string) bool {
		if colors[id] == 1 {
			return false
		}
		if colors[id] == 2 {
			return true
		}
		colors[id] = 1
		for _, to := range forward[id] {
			if !acyclic(to) {
				return false
			}
		}
		colors[id] = 2
		return true
	}
	if !acyclic(entry) {
		return nil, errors.New("Success routes must be acyclic; use bounded failure routes for feedback.")
	}
	for _, edge := range p.Edges {
		if edge.Condition != "failure" {
			continue
		}
		visited := map[string]bool{}
		var walk func(string)
		walk = func(id string) {
			if visited[id] {
				return
			}
			visited[id] = true
			for _, to := range forward[id] {
				walk(to)
			}
		}
		walk(edge.To)
		if !visited[edge.From] {
			return nil, errors.New("Repair routes must return to an upstream task.")
		}
	}
	return &p, nil
}
