// Package skills manages project-installed skill definitions without executing them.
package skills

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/goccy/go-yaml"
)

const (
	maxFileBytes        = 256 << 10
	maxFrontMatterBytes = 16 << 10
	maxEntries          = 4096
	maxSkills           = 512
	maxDepth            = 8
)

var (
	ErrUnavailable = errors.New("The skill file is unavailable.")
	ErrTooLarge    = errors.New("The skill file is too large to preview.")
	ErrInvalidText = errors.New("The skill file is not UTF-8 text.")
	ErrNotFound    = errors.New("Skill not found. Refresh the project skills.")
)

var roots = []string{".agents/skills", ".claude/skills", ".codex/skills", "skills"}

type Skill struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Paths       []string `json:"paths"`
	Error       string   `json:"error,omitempty"`
	Editable    bool     `json:"editable"`
	Deletable   bool     `json:"deletable"`
	Revision    string   `json:"revision"`
}

type Catalog struct {
	Skills     []Skill `json:"skills"`
	Limited    bool    `json:"limited"`
	Unreadable bool    `json:"unreadable"`
}

type Detail struct {
	Skill
	Content  string `json:"content"`
	Source   string `json:"source"`
	Hash     string `json:"hash"`
	ScopeKey string `json:"scopeKey"`
}

type definition struct {
	skill Skill
	path  string
}

type scanner struct {
	ctx         context.Context
	root        string
	entries     int
	definitions []definition
	files       map[string]int
	limited     bool
	unreadable  bool
}

func List(ctx context.Context, directory string) (Catalog, error) {
	s, err := scan(ctx, directory)
	if err != nil {
		return Catalog{}, err
	}
	result := Catalog{Skills: []Skill{}, Limited: s.limited, Unreadable: s.unreadable}
	for _, value := range s.definitions {
		result.Skills = append(result.Skills, value.skill)
	}
	sort.SliceStable(result.Skills, func(i, j int) bool {
		a, b := strings.ToLower(result.Skills[i].Name), strings.ToLower(result.Skills[j].Name)
		if a == b {
			return result.Skills[i].Paths[0] < result.Skills[j].Paths[0]
		}
		return a < b
	})
	return result, nil
}

// Read accepts only an identity rediscovered in this project's skill roots.
func Read(ctx context.Context, directory, id string) (Detail, error) {
	s, err := scan(ctx, directory)
	if err != nil {
		return Detail{}, err
	}
	for _, value := range s.definitions {
		if value.skill.ID != id {
			continue
		}
		if err := ctx.Err(); err != nil {
			return Detail{}, err
		}
		raw, err := readFile(value.path)
		if err != nil {
			return Detail{}, err
		}
		name, description, body := parse(raw, value.skill.Name)
		value.skill.Name, value.skill.Description, value.skill.Error = name, description, ""
		return Detail{Skill: value.skill, Content: body, Source: string(raw), Hash: sourceHash(raw)}, nil
	}
	return Detail{}, ErrNotFound
}

func scan(ctx context.Context, directory string) (*scanner, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	root, err := filepath.EvalSymlinks(directory)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !filepath.IsAbs(root) || !info.IsDir() {
		return nil, ErrUnavailable
	}
	s := &scanner{ctx: ctx, root: root, files: make(map[string]int)}
	for _, relative := range roots {
		if err := s.walk(filepath.Join(root, filepath.FromSlash(relative)), 0, map[string]bool{}); err != nil {
			return nil, err
		}
	}
	for index := range s.definitions {
		value := &s.definitions[index]
		plan, err := planManagement(s.root, *value)
		if err == nil {
			value.skill.Editable = plan.edit != "" && value.skill.Error == ""
			value.skill.Deletable = len(plan.remove) > 0
			value.skill.Revision = plan.revision
		}
	}
	return s, nil
}

func (s *scanner) walk(directory string, depth int, ancestors map[string]bool) error {
	if err := s.ctx.Err(); err != nil {
		return err
	}
	if depth > maxDepth || s.entries >= maxEntries {
		s.limited = true
		return nil
	}
	resolved, err := filepath.EvalSymlinks(directory)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			s.unreadable = true
		}
		return nil
	}
	if ancestors[resolved] {
		return nil
	}
	ancestors[resolved] = true
	defer delete(ancestors, resolved)
	folder, err := os.Open(resolved)
	if err != nil {
		s.unreadable = true
		return nil
	}
	entries, readErr := folder.ReadDir(maxEntries - s.entries + 1)
	folder.Close()
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		s.unreadable = true
		return nil
	}
	if len(entries) > maxEntries-s.entries {
		entries = entries[:maxEntries-s.entries]
		s.limited = true
	}
	s.entries += len(entries)
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		if entry.Name() == "SKILL.md" {
			s.add(filepath.Join(directory, "SKILL.md"))
			// Supporting assets and nested examples belong to this definition.
			return nil
		}
	}
	for _, entry := range entries {
		if entry.Name() == ".git" || entry.Name() == "node_modules" {
			continue
		}
		target := filepath.Join(directory, entry.Name())
		dir := entry.IsDir()
		if entry.Type()&os.ModeSymlink != 0 {
			info, err := os.Stat(target)
			if err != nil {
				s.unreadable = true
				continue
			}
			dir = info.IsDir()
		}
		if dir {
			if err := s.walk(target, depth+1, ancestors); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *scanner) add(path string) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		s.unreadable = true
		return
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.Mode().IsRegular() {
		s.unreadable = true
		return
	}
	relative, err := filepath.Rel(s.root, path)
	if err != nil {
		return
	}
	relative = filepath.ToSlash(relative)
	if index, ok := s.files[resolved]; ok {
		s.definitions[index].skill.Paths = append(s.definitions[index].skill.Paths, relative)
		return
	}
	if len(s.definitions) >= maxSkills {
		s.limited = true
		return
	}
	hash := sha256.Sum256([]byte(resolved))
	skill := Skill{ID: hex.EncodeToString(hash[:16]), Name: filepath.Base(filepath.Dir(path)), Paths: []string{relative}}
	raw, err := readFile(resolved)
	if err != nil {
		skill.Error = err.Error()
		s.unreadable = true
	} else {
		skill.Name, skill.Description, _ = parse(raw, skill.Name)
	}
	s.files[resolved] = len(s.definitions)
	s.definitions = append(s.definitions, definition{skill: skill, path: resolved})
}

func readFile(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return nil, ErrUnavailable
	}
	if info.Size() > maxFileBytes {
		return nil, ErrTooLarge
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, ErrUnavailable
	}
	defer file.Close()
	info, err = file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, ErrUnavailable
	}
	raw, err := io.ReadAll(io.LimitReader(file, maxFileBytes+1))
	if err != nil {
		return nil, ErrUnavailable
	}
	if len(raw) > maxFileBytes {
		return nil, ErrTooLarge
	}
	if !utf8.Valid(raw) {
		return nil, ErrInvalidText
	}
	return raw, nil
}

func parse(raw []byte, fallback string) (name, description, body string) {
	body = strings.TrimPrefix(string(raw), "\ufeff")
	name = fallback
	lines := strings.SplitAfter(body, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return
	}
	end := len(lines[0])
	for _, line := range lines[1:] {
		if end > maxFrontMatterBytes {
			return
		}
		if strings.TrimSpace(line) == "---" {
			var metadata struct {
				Name        string `yaml:"name"`
				Description string `yaml:"description"`
			}
			if yaml.Unmarshal([]byte(body[len(lines[0]):end]), &metadata) != nil {
				return
			}
			if value := strings.TrimSpace(metadata.Name); value != "" {
				name = clip(value, 120)
			}
			var display struct {
				Metadata struct {
					DisplayName string `yaml:"display_name"`
				} `yaml:"metadata"`
			}
			if yaml.Unmarshal([]byte(body[len(lines[0]):end]), &display) == nil {
				if value := strings.TrimSpace(display.Metadata.DisplayName); value != "" {
					name = clip(value, 128)
				}
			}
			description = clip(strings.TrimSpace(metadata.Description), 2048)
			body = body[end+len(line):]
			return
		}
		end += len(line)
	}
	return
}

func clip(value string, limit int) string {
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	return string([]rune(value)[:limit])
}
