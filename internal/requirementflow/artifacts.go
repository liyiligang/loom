// Package requirementflow validates the preparation protocol independently of
// native agent transport. Only actual bounded files can advance the workflow.
package requirementflow

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"

	"github.com/liyiligang/loom/internal/materials"
)

type Question struct {
	ID       string `json:"id"`
	Question string `json:"question"`
	Reason   string `json:"reason"`
}
type Receipt struct {
	SchemaVersion int        `json:"schemaVersion"`
	WorkflowID    string     `json:"workflowId"`
	Stage         string     `json:"stage"`
	RequestID     string     `json:"requestId"`
	AcceptanceIDs []string   `json:"acceptanceIds"`
	ResolvedIDs   []string   `json:"resolvedIds"`
	Questions     []Question `json:"questions"`
}
type Document struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Hash    string `json:"hash"`
}

var DocumentPaths = []string{"documents/requirements.md", "design-documents/design.md", "decisions/grill-me.md"}
var stableID = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,63}$`)

func Hash(value []byte) string { sum := sha256.Sum256(value); return hex.EncodeToString(sum[:]) }
func ReviewHash(scopeKey, commit string, hashes map[string]string) string {
	value, _ := json.Marshal([]any{scopeKey, commit, hashes})
	return Hash(value)
}
func ReceiptPath(id, requestID string) string { return path.Join(id, "receipts", requestID+".json") }
func ReadReceipt(scope materials.Scope, id, stage, requestID string) (Receipt, error) {
	var receipt Receipt
	raw, err := materials.ReadWorkflow(scope, ReceiptPath(id, requestID))
	if err != nil {
		return receipt, fmt.Errorf("Stage receipt unavailable: %w", err)
	}
	if err := json.Unmarshal(raw, &receipt); err != nil {
		return receipt, errors.New("The stage receipt is not valid JSON.")
	}
	if receipt.SchemaVersion != 1 || receipt.WorkflowID != id || receipt.Stage != stage || receipt.RequestID != requestID {
		return receipt, errors.New("The stage receipt does not identify this workflow attempt.")
	}
	return receipt, nil
}
func ReadDocuments(ctx context.Context, scope materials.Scope, count int) ([]Document, map[string]string, error) {
	catalog, err := materials.Scan(ctx, scope)
	if err != nil {
		return nil, nil, err
	}
	documents, hashes := []Document{}, map[string]string{}
	for _, relative := range DocumentPaths[:count] {
		file, err := materials.Find(catalog, materials.FileID(scope, path.Join(scope.Root, relative)))
		if err != nil {
			return nil, nil, fmt.Errorf("Required document is missing: %s", relative)
		}
		detail, err := materials.Read(scope, file)
		if err != nil {
			return nil, nil, err
		}
		if detail.Truncated || len(strings.TrimSpace(detail.Content)) < 20 {
			return nil, nil, fmt.Errorf("Required document must contain substantive text: %s", relative)
		}
		hashes[relative] = detail.Hash
		documents = append(documents, Document{relative, detail.Content, detail.Hash})
	}
	return documents, hashes, nil
}
func ValidateIDs(ids []string, minimum, maximum int) error {
	if len(ids) < minimum || len(ids) > maximum {
		return errors.New("Invalid number of acceptance criteria or question identifiers.")
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if !stableID.MatchString(id) || seen[id] {
			return errors.New("Identifiers must be unique ASCII identifiers of at most 64 characters.")
		}
		seen[id] = true
	}
	return nil
}
func Covers(actual, expected []string) bool {
	if ValidateIDs(actual, 0, 100) != nil {
		return false
	}
	set := map[string]bool{}
	for _, id := range actual {
		set[id] = true
	}
	for _, id := range expected {
		if !set[id] {
			return false
		}
	}
	return true
}
func ValidateQuestions(questions []Question) error {
	ids := []string{}
	for _, q := range questions {
		ids = append(ids, q.ID)
		if strings.TrimSpace(q.Question) == "" || len(q.Question) > 2000 || strings.TrimSpace(q.Reason) == "" || len(q.Reason) > 2000 {
			return errors.New("Each Grill me question needs a bounded question and reason.")
		}
	}
	return ValidateIDs(ids, 0, 8)
}
