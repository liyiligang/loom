package skills

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateAndUpdatePreserveSourceAndAssets(t *testing.T) {
	ctx, root := context.Background(), t.TempDir()
	source := "---\nname: review\ndescription: Review project changes.\ncustom: retained\n---\n# Review\n\nKeep this workflow.\n"
	detail, err := Create(ctx, root, "review", source)
	if err != nil || detail.Source != source || !detail.Editable || !detail.Deletable || detail.Hash == "" {
		t.Fatalf("create: %+v %v", detail, err)
	}
	file := filepath.Join(root, ".agents/skills/review/SKILL.md")
	asset := filepath.Join(root, ".agents/skills/review/script.txt")
	if err := os.WriteFile(asset, []byte("Keep supporting file"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(file, 0600); err != nil {
		t.Fatal(err)
	}
	updated := strings.Replace(source, "Keep this workflow.", "Updated workflow.", 1)
	after, err := Update(ctx, root, detail.ID, updated, detail.Hash)
	if err != nil || after.Source != updated || after.ID != detail.ID {
		t.Fatalf("update: %+v %v", after, err)
	}
	info, _ := os.Stat(file)
	if info.Mode().Perm() != 0600 {
		t.Fatal("update changed existing file permissions")
	}
	if raw, err := os.ReadFile(asset); err != nil || string(raw) != "Keep supporting file" {
		t.Fatal("update removed supporting files")
	}
	if _, err := Update(ctx, root, detail.ID, source, detail.Hash); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale update: %v", err)
	}
	if err := Delete(ctx, root, detail.ID, detail.Revision); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale deletion: %v", err)
	}
	if _, err := Create(ctx, root, "review", source); !errors.Is(err, ErrConflict) {
		t.Fatal("creation replaced an installed skill")
	}
	entries, _ := os.ReadDir(filepath.Dir(file))
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".loom-") {
			t.Fatal("temporary file remained")
		}
	}
}

func TestDeleteRemovesOwnedFolderAndAliasesWithoutTouchingOtherSkills(t *testing.T) {
	ctx, root := context.Background(), t.TempDir()
	file := writeSkill(t, root, ".agents/skills/shared", "---\nname: shared\n---\nBody")
	other := writeSkill(t, root, ".agents/skills/other", "Other")
	asset := filepath.Join(filepath.Dir(file), "assets", "input.txt")
	if err := os.MkdirAll(filepath.Dir(asset), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(asset, []byte("Asset"), 0644); err != nil {
		t.Fatal(err)
	}
	claude := filepath.Join(root, ".claude/skills/shared")
	codex := filepath.Join(root, ".codex/skills/shared")
	linkSkill(t, filepath.Dir(file), claude)
	linkSkill(t, filepath.Dir(file), codex)
	catalog, err := List(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	var shared Skill
	for _, skill := range catalog.Skills {
		if skill.Name == "shared" {
			shared = skill
		}
	}
	if !shared.Editable || !shared.Deletable || len(shared.Paths) != 3 {
		t.Fatalf("alias capabilities: %+v", shared)
	}
	if err := Delete(ctx, root, shared.ID, shared.Revision); err != nil {
		t.Fatal(err)
	}
	for _, removed := range []string{filepath.Dir(file), claude, codex} {
		if _, err := os.Lstat(removed); !os.IsNotExist(err) {
			t.Fatalf("installation remained: %s %v", removed, err)
		}
	}
	if raw, err := os.ReadFile(other); err != nil || string(raw) != "Other" {
		t.Fatal("another skill was affected")
	}
	catalog, err = List(ctx, root)
	if err != nil || len(catalog.Skills) != 1 || catalog.Unreadable {
		t.Fatalf("remaining catalog: %+v %v", catalog, err)
	}
}

func TestLinkedSourcesAndSharedRootsStayOutsideMutationScope(t *testing.T) {
	ctx, root, external := context.Background(), t.TempDir(), t.TempDir()
	file := writeSkill(t, external, "external", "External source")
	link := filepath.Join(root, ".agents/skills/external")
	linkSkill(t, filepath.Dir(file), link)
	catalog, err := List(ctx, root)
	if err != nil || len(catalog.Skills) != 1 || catalog.Skills[0].Editable || !catalog.Skills[0].Deletable {
		t.Fatalf("external capabilities: %+v %v", catalog, err)
	}
	detail, err := Read(ctx, root, catalog.Skills[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Update(ctx, root, detail.ID, "Wrong project", detail.Hash); !errors.Is(err, ErrReadOnly) {
		t.Fatal("external source was writable")
	}
	if err := Delete(ctx, root, detail.ID, detail.Revision); err != nil {
		t.Fatal(err)
	}
	if raw, err := os.ReadFile(file); err != nil || string(raw) != "External source" {
		t.Fatal("external source was deleted")
	}
	sharedRoot := t.TempDir()
	linkSkill(t, external, filepath.Join(sharedRoot, ".agents/skills"))
	catalog, err = List(ctx, sharedRoot)
	if err != nil || len(catalog.Skills) != 1 || catalog.Skills[0].Editable || catalog.Skills[0].Deletable {
		t.Fatalf("shared-root capabilities: %+v %v", catalog, err)
	}
	if _, err := Create(ctx, sharedRoot, "new-skill", "New"); !errors.Is(err, ErrReadOnly) {
		t.Fatal("create followed shared root")
	}
	if err := Delete(ctx, sharedRoot, catalog.Skills[0].ID, catalog.Skills[0].Revision); !errors.Is(err, ErrReadOnly) {
		t.Fatal("delete followed shared root")
	}
}

func TestRootDefinitionDeletionAndInvalidInput(t *testing.T) {
	ctx, root := context.Background(), t.TempDir()
	file := writeSkill(t, root, ".agents/skills", "Root definition")
	asset := filepath.Join(filepath.Dir(file), "unrelated.txt")
	if err := os.WriteFile(asset, []byte("Preserve"), 0644); err != nil {
		t.Fatal(err)
	}
	catalog, err := List(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if err := Delete(ctx, root, catalog.Skills[0].ID, catalog.Skills[0].Revision); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(asset); err != nil {
		t.Fatal("root deletion removed siblings")
	}
	for _, name := range []string{"../escape", "bad_name", "Bad", "a--b", strings.Repeat("a", 65)} {
		if _, err := Create(ctx, root, name, "Body"); !errors.Is(err, ErrInvalidName) {
			t.Fatalf("invalid name %s: %v", name, err)
		}
	}
	for _, content := range []string{"", "\xff", "a\x00b", strings.Repeat("x", maxFileBytes+1), "---\nname: [\n---\nBody", "---\nname: missing-end\n"} {
		if _, err := Create(ctx, root, "invalid", content); !errors.Is(err, ErrInvalidSource) {
			t.Fatalf("invalid source accepted: %v", err)
		}
	}
	if _, err := Update(ctx, root, "/arbitrary/path", "Body", "hash"); !errors.Is(err, ErrNotFound) {
		t.Fatal("arbitrary path accepted")
	}
}
