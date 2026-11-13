package model

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/liyiligang/loom/internal/workspace"
)

// BranchGroup owns member identities, independently of the material source.
type BranchGroup struct {
	ID                string         `json:"id"`
	Prefix            string         `json:"prefix"`
	RepositoryRoot    string         `json:"repositoryRoot"`
	CommonDir         string         `json:"commonDir"`
	BaseRef           string         `json:"baseRef"`
	BaseCommit        string         `json:"baseCommit"`
	Members           []BranchMember `json:"members"`
	MaterialBranchID  string         `json:"materialBranchId,omitempty"`
	ExplicitSelection bool           `json:"explicitSelection"`
}

type BranchMember struct {
	ID        string            `json:"id"`
	Workspace workspace.Binding `json:"workspace"`
}

func BranchMemberID(requirementID string, binding workspace.Binding) string {
	hash := sha256.Sum256([]byte(requirementID + "\x00" + binding.CommonDir + "\x00" + binding.Branch))
	return hex.EncodeToString(hash[:16])
}

func (g *BranchGroup) Member(id string) *BranchMember {
	if g != nil {
		for index := range g.Members {
			if g.Members[index].ID == id {
				return &g.Members[index]
			}
		}
	}
	return nil
}

func (g *BranchGroup) MemberFor(binding workspace.Binding) *BranchMember {
	if g != nil {
		for index := range g.Members {
			member := &g.Members[index]
			if member.Workspace.CommonDir == binding.CommonDir && member.Workspace.Branch == binding.Branch {
				return member
			}
		}
	}
	return nil
}

// SyncBranchGroup preserves the legacy v1 binding projection during migration.
// New group operations select execution members explicitly; Workspace remains
// the compatibility projection of the requirement's material source.
func (r *Requirement) SyncBranchGroup() {
	legacy := append([]workspace.Binding(nil), r.PreviousWorkspaces...)
	if r.Workspace != nil {
		legacy = append(legacy, *r.Workspace)
	}
	if r.BranchGroup == nil {
		if len(legacy) == 0 {
			return
		}
		selected := legacy[len(legacy)-1]
		r.BranchGroup = &BranchGroup{ID: r.ID, Prefix: "feat/requirement-" + r.ID,
			RepositoryRoot: selected.RepositoryRoot, CommonDir: selected.CommonDir,
			BaseRef: selected.BaseRef, BaseCommit: selected.BaseCommit, Members: []BranchMember{}}
	}
	if r.BranchGroup.Members == nil {
		r.BranchGroup.Members = []BranchMember{}
	}
	if !r.BranchGroup.ExplicitSelection {
		// Legacy callers replace the compatibility projection directly. Rebuild it
		// so a rejected transaction cannot retain an uncommitted branch member.
		r.BranchGroup.Members = []BranchMember{}
	}
	for _, binding := range legacy {
		if member := r.BranchGroup.MemberFor(binding); member != nil {
			member.Workspace = binding
		} else {
			r.BranchGroup.Members = append(r.BranchGroup.Members, BranchMember{ID: BranchMemberID(r.ID, binding), Workspace: binding})
		}
	}
	if r.Workspace != nil {
		r.BranchGroup.MaterialBranchID = r.BranchGroup.MemberFor(*r.Workspace).ID
	}
}

func (r *Requirement) MaterialWorkspace() *workspace.Binding {
	if r.BranchGroup != nil {
		if member := r.BranchGroup.Member(r.BranchGroup.MaterialBranchID); member != nil {
			return &member.Workspace
		}
		return nil
	}
	return r.Workspace
}

func (r *Requirement) SetMemberState(id, state, message string) {
	member := r.BranchGroup.Member(id)
	if member == nil {
		return
	}
	member.Workspace.State, member.Workspace.Error = state, message
	update := func(binding *workspace.Binding) {
		if binding != nil && binding.CommonDir == member.Workspace.CommonDir && binding.Branch == member.Workspace.Branch {
			binding.State, binding.Error = state, message
		}
	}
	update(r.Workspace)
	for index := range r.PreviousWorkspaces {
		update(&r.PreviousWorkspaces[index])
	}
}
