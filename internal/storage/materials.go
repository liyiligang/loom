package storage

import (
	"github.com/liyiligang/loom/internal/materials"
	"github.com/liyiligang/loom/internal/model"
)

// SyncMaterials stores only metadata. Files remain the authoritative content.
// Upserts preserve links; disappearance removes only the current scope's index.
func (s *Store) SyncMaterials(catalog *materials.Catalog) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	scope := catalog.Scope
	scan := model.ID()
	for _, file := range catalog.Files {
		_, err = tx.Exec(`INSERT INTO requirement_materials VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(requirement_id,scope,id) DO UPDATE SET path=excluded.path,kind=excluded.kind,title=excluded.title,directory=excluded.directory,branch=excluded.branch,commit_id=excluded.commit_id,hash=excluded.hash,size=excluded.size,modified_at=excluded.modified_at,scan_id=excluded.scan_id`, scope.RequirementID, scope.ProjectID, scope.Key, file.ID, file.Path, file.Kind, file.Title, scope.Directory, scope.Branch, scope.Commit, file.Hash, file.Size, file.ModifiedAt, scan)
		if err != nil {
			return err
		}
	}
	if !catalog.Limited {
		if _, err := tx.Exec("DELETE FROM requirement_materials WHERE requirement_id=? AND scope=? AND scan_id<>?", scope.RequirementID, scope.Key, scan); err != nil {
			return err
		}
	}
	rows, err := tx.Query("SELECT source_id,target_id FROM material_links WHERE requirement_id=? AND scope=? ORDER BY target_id", scope.RequirementID, scope.Key)
	if err != nil {
		return err
	}
	links := make(map[string][]string)
	for rows.Next() {
		var source, target string
		if err := rows.Scan(&source, &target); err != nil {
			rows.Close()
			return err
		}
		links[source] = append(links[source], target)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for index := range catalog.Files {
		catalog.Files[index].RelatedIDs = []string{}
		if values := links[catalog.Files[index].ID]; values != nil {
			catalog.Files[index].RelatedIDs = values
		}
	}
	return tx.Commit()
}
func (s *Store) SetMaterialLinks(scope materials.Scope, id string, targets []string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec("DELETE FROM material_links WHERE requirement_id=? AND scope=? AND source_id=?", scope.RequirementID, scope.Key, id); err != nil {
		return err
	}
	for _, target := range targets {
		if _, err := tx.Exec("INSERT INTO material_links VALUES (?,?,?,?)", scope.RequirementID, scope.Key, id, target); err != nil {
			return err
		}
	}
	return tx.Commit()
}
