package skills

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeSkill(t *testing.T, root, relative, content string) string {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative), "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func linkSkill(t *testing.T, target, link string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(link), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
}

func TestMetadataAndMarkdown(t *testing.T) {
	for _, test := range []struct{ label, raw, name, description, body string }{
		{"quoted", "---\nname: 'Review: API'\ndescription: \"Check: code # changes\"\n---\n# Review\n", "Review: API", "Check: code # changes", "# Review\n"},
		{"folded", "---\nname: review\ndescription: >-\n  Check code\n  and behavior.\n---\nBody", "review", "Check code and behavior.", "Body"},
		{"bom-crlf", "\ufeff---\r\nname: test\r\ndescription: Read files.\r\n---\r\nBody\r\n", "test", "Read files.", "Body\r\n"},
		{"plain", "# Plain\n", "fallback", "", "# Plain\n"},
		{"missing-close", "---\nname: ignored\n", "fallback", "", "---\nname: ignored\n"},
		{"invalid", "---\nname: [invalid\n---\nBody", "fallback", "", "---\nname: [invalid\n---\nBody"},
		{"blank-name", "---\nname: ' '\ndescription: ' useful '\n---\nBody", "fallback", "useful", "Body"},
	} {
		t.Run(test.label, func(t *testing.T) {
			root := t.TempDir()
			writeSkill(t, root, ".agents/skills/fallback", test.raw)
			catalog, err := List(context.Background(), root)
			if err != nil || len(catalog.Skills) != 1 {
				t.Fatalf("catalog: %+v, %v", catalog, err)
			}
			skill := catalog.Skills[0]
			if skill.Name != test.name || skill.Description != test.description {
				t.Fatalf("metadata: %+v", skill)
			}
			detail, err := Read(context.Background(), root, skill.ID)
			if err != nil || detail.Content != test.body {
				t.Fatalf("body: %q, %v", detail.Content, err)
			}
		})
	}
}

func TestInstalledAliasesNestedSkillsAndCycles(t *testing.T) {
	root, external := t.TempDir(), t.TempDir()
	path := writeSkill(t, root, ".agents/skills/review", "---\nname: review\n---\nShared")
	writeSkill(t, root, ".agents/skills/review/examples/example", "Hidden supporting example")
	writeSkill(t, root, ".claude/skills/review-copy", "---\nname: review\n---\nSeparate copy")
	writeSkill(t, root, ".codex/skills/category/nested", "Nested definition")
	writeSkill(t, root, "skills/simple", "Plain definition")
	writeSkill(t, external, "installed", "External installed definition")
	linkSkill(t, filepath.Dir(path), filepath.Join(root, ".claude/skills/review"))
	linkSkill(t, filepath.Dir(path), filepath.Join(root, ".codex/skills/review"))
	linkSkill(t, filepath.Join(external, "installed"), filepath.Join(root, "skills/external"))
	linkSkill(t, filepath.Join(root, ".codex/skills"), filepath.Join(root, ".codex/skills/category/cycle"))
	catalog, err := List(context.Background(), root)
	if err != nil || len(catalog.Skills) != 5 || catalog.Limited || catalog.Unreadable {
		t.Fatalf("catalog: %+v, %v", catalog, err)
	}
	var shared Skill
	for _, skill := range catalog.Skills {
		if len(skill.Paths) == 3 {
			shared = skill
		}
		if skill.Name == "example" {
			t.Fatal("supporting example became an independent skill")
		}
	}
	if shared.Name != "review" || shared.Paths[0] != ".agents/skills/review/SKILL.md" {
		t.Fatalf("aliases: %+v", shared)
	}
	refreshed, _ := List(context.Background(), root)
	for i := range catalog.Skills {
		if refreshed.Skills[i].ID != catalog.Skills[i].ID {
			t.Fatal("unchanged files changed identity or ordering")
		}
	}
}

func TestReadStaysWithinScopeAndRefreshesChanges(t *testing.T) {
	root, other := t.TempDir(), t.TempDir()
	path := writeSkill(t, root, "skills/example", "Before")
	writeSkill(t, other, "skills/example", "Other project")
	catalog, _ := List(context.Background(), root)
	id := catalog.Skills[0].ID
	if _, err := Read(context.Background(), other, id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-project identity accepted: %v", err)
	}
	if _, err := Read(context.Background(), root, path); !errors.Is(err, ErrNotFound) {
		t.Fatalf("arbitrary file path accepted: %v", err)
	}
	if err := os.WriteFile(path, []byte("After"), 0644); err != nil {
		t.Fatal(err)
	}
	detail, err := Read(context.Background(), root, id)
	if err != nil || detail.Content != "After" {
		t.Fatalf("stale preview: %+v, %v", detail, err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(context.Background(), root, id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("removed skill accepted: %v", err)
	}
	refreshed, err := List(context.Background(), root)
	if err != nil || len(refreshed.Skills) != 0 {
		t.Fatalf("deleted skill remained listed: %+v, %v", refreshed, err)
	}
}

func TestUnavailableAndInvalidDefinitions(t *testing.T) {
	for _, test := range []struct {
		name, content string
		want          error
	}{
		{"oversized", strings.Repeat("a", maxFileBytes+1), ErrTooLarge},
		{"invalid-utf8", string([]byte{0xff}), ErrInvalidText},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			writeSkill(t, root, "skills/example", test.content)
			catalog, err := List(context.Background(), root)
			if err != nil || len(catalog.Skills) != 1 || !catalog.Unreadable || catalog.Skills[0].Error != test.want.Error() {
				t.Fatalf("invalid file not visible: %+v, %v", catalog, err)
			}
			if _, err := Read(context.Background(), root, catalog.Skills[0].ID); !errors.Is(err, test.want) {
				t.Fatalf("invalid preview: %v", err)
			}
		})
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "skills/example/SKILL.md"), 0755); err != nil {
		t.Fatal(err)
	}
	catalog, err := List(context.Background(), root)
	if err != nil || len(catalog.Skills) != 0 || !catalog.Unreadable {
		t.Fatalf("directory accepted as Markdown: %+v, %v", catalog, err)
	}
}

func TestScanLimitsEmptyScopeAndCancellation(t *testing.T) {
	root := t.TempDir()
	empty, err := List(context.Background(), root)
	if err != nil || empty.Skills == nil || len(empty.Skills) != 0 || empty.Limited || empty.Unreadable {
		t.Fatalf("empty project: %+v, %v", empty, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := List(ctx, root); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
	if _, err := List(context.Background(), filepath.Join(root, "missing")); err == nil {
		t.Fatal("missing project accepted")
	}
	for i := 0; i <= maxSkills; i++ {
		writeSkill(t, root, fmt.Sprintf("skills/item-%04d", i), "Body")
	}
	limited, err := List(context.Background(), root)
	if err != nil || !limited.Limited || len(limited.Skills) != maxSkills {
		t.Fatalf("skill count limit: %d, %v, %v", len(limited.Skills), limited.Limited, err)
	}
	deep := t.TempDir()
	writeSkill(t, deep, "skills/"+strings.Repeat("nested/", maxDepth+1)+"example", "Deep")
	limited, err = List(context.Background(), deep)
	if err != nil || !limited.Limited || len(limited.Skills) != 0 {
		t.Fatalf("depth limit: %+v, %v", limited, err)
	}
	entries := t.TempDir()
	for i := 0; i <= maxEntries; i++ {
		path := filepath.Join(entries, fmt.Sprintf("skills/item-%04d", i))
		if err := os.MkdirAll(path, 0755); err != nil {
			t.Fatal(err)
		}
	}
	limited, err = List(context.Background(), entries)
	if err != nil || !limited.Limited {
		t.Fatalf("entry limit: %+v, %v", limited, err)
	}
}
