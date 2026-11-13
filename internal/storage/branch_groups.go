package storage

import (
	"database/sql"
	"encoding/json"

	"github.com/liyiligang/loom/internal/model"
)

// migrateBranchGroups extends metadata only; native identities and files stay put.
func migrateBranchGroups(tx *sql.Tx) error {
	rows, err := tx.Query("SELECT data FROM requirements ORDER BY rowid")
	if err != nil {
		return err
	}
	requirements := map[string]*model.Requirement{}
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			rows.Close()
			return err
		}
		var requirement model.Requirement
		if err := json.Unmarshal(raw, &requirement); err != nil {
			rows.Close()
			return err
		}
		requirement.SyncBranchGroup()
		requirements[requirement.ID] = &requirement
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, requirement := range requirements {
		if requirement.BranchGroup != nil {
			if err := save(tx, requirement); err != nil {
				return err
			}
		}
	}
	rows, err = tx.Query("SELECT data FROM sessions ORDER BY rowid")
	if err != nil {
		return err
	}
	sessions := []*model.Session{}
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			rows.Close()
			return err
		}
		var session model.Session
		if err := json.Unmarshal(raw, &session); err != nil {
			rows.Close()
			return err
		}
		if requirement := requirements[session.RequirementID]; requirement != nil && session.Workspace != nil {
			if member := requirement.BranchGroup.MemberFor(*session.Workspace); member != nil {
				session.BranchID = member.ID
				sessions = append(sessions, &session)
			}
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, session := range sessions {
		if err := save(tx, session); err != nil {
			return err
		}
	}
	return nil
}
