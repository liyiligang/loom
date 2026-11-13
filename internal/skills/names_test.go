package skills

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
)

func TestUnicodeSkillNamesPreserveNativeIdentifiersAndAssets(t *testing.T) {
	ctx, directory := context.Background(), t.TempDir()
	name := "\u9700\u6c42\u5bf9\u9f50"
	body := "\n# Instructions\n\nRead requirements before designing.\n"
	source := "---\nname: new-skill\n# Preserve this comment\ndescription: Align documents.\ncustom: retained\nmetadata:\n  category: planning\n---\n" + body
	created, err := CreateNamed(ctx, directory, name, source)
	if err != nil {
		t.Fatal(err)
	}
	if created.Name != name || !strings.HasSuffix(created.Source, body) || !strings.Contains(created.Source, "# Preserve this comment") || !strings.Contains(created.Source, "custom: retained") {
		t.Fatalf("display name or instructions changed: %+v", created)
	}
	identifier := installationName(name)
	if !skillName.MatchString(identifier) || created.Paths[0] != ".agents/skills/"+identifier+"/SKILL.md" {
		t.Fatal("invalid generated identifier")
	}
	var metadata struct {
		Name     string            `yaml:"name"`
		Metadata map[string]string `yaml:"metadata"`
	}
	if err := yaml.Unmarshal([]byte(splitSource(created.Source).metadata), &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata.Name != identifier || metadata.Metadata["category"] != "planning" || metadata.Metadata["display_name"] != name {
		t.Fatal("metadata was not preserved")
	}
	asset := filepath.Join(directory, filepath.Dir(filepath.FromSlash(created.Paths[0])), "input.txt")
	if err := os.WriteFile(asset, []byte("Supporting input"), 0644); err != nil {
		t.Fatal(err)
	}
	renamed, err := UpdateNamed(ctx, directory, created.ID, "\u8bbe\u8ba1\u5bf9\u9f50", created.Source, created.Hash)
	if err != nil {
		t.Fatal(err)
	}
	if renamed.Name == name || renamed.ID != created.ID || renamed.Paths[0] != created.Paths[0] || nativeName(renamed.Source, "") != identifier {
		t.Fatal("renaming changed native identity")
	}
	if raw, err := os.ReadFile(asset); err != nil || string(raw) != "Supporting input" {
		t.Fatal("renaming lost supporting files")
	}
	if _, err := CreateNamed(ctx, directory, name, source); !errors.Is(err, ErrConflict) {
		t.Fatal("duplicate creation replaced a skill")
	}
}

func TestDisplayNameValidationDoesNotUseNamesAsPaths(t *testing.T) {
	ctx, directory := context.Background(), t.TempDir()
	for _, name := range []string{"", " \t ", "line\nbreak", "null\x00name", "\xff", strings.Repeat("x", 129)} {
		if _, err := CreateNamed(ctx, directory, name, "Instructions"); !errors.Is(err, ErrInvalidDisplayName) {
			t.Fatalf("invalid name accepted: %q %v", name, err)
		}
	}
	for _, name := range []string{"../escape", "API Review", "\u9700\u6c42 / \u8bbe\u8ba1", strings.Repeat("\u5b57", 128)} {
		value, err := CreateNamed(ctx, directory, name, "Instructions")
		if err != nil {
			t.Fatal(err)
		}
		if value.Name != name || strings.Contains(value.Paths[0], "..") || !strings.HasPrefix(value.Paths[0], ".agents/skills/skill-") {
			t.Fatal("display text affected a filesystem path")
		}
	}
}

func TestNamedSourcesPreserveLegacyAndMalformedDefinitions(t *testing.T) {
	source := "---\nname: api-review\ncustom: preserve\n---\nBody"
	if result, err := namedSource(source, "api-review", "api-review"); err != nil || result != source {
		t.Fatal("unchanged legacy source was rewritten")
	}
	for _, source := range []string{
		"---\n{name: old, metadata: {category: planning}, description: Test}\n---\nBody",
		"\ufeff---\r\nname: old\r\nmetadata: null\r\n---\r\nBody\r\n",
	} {
		result, err := namedSource(source, "\u5bf9\u9f50", "native-skill")
		if err != nil {
			t.Fatal(err)
		}
		if nativeName(result, "") != "native-skill" || !strings.Contains(result, "Body") {
			t.Fatal("native name or body lost")
		}
	}
	for _, source := range []string{"---\nname: [\n---\nBody", "---\nmetadata: replace-me\n---\nBody"} {
		if _, err := namedSource(source, "\u5bf9\u9f50", "native-skill"); !errors.Is(err, ErrInvalidSource) {
			t.Fatal("invalid metadata was overwritten")
		}
	}
}
