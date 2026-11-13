package skills

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/goccy/go-yaml"
)

var (
	ErrConflict           = errors.New("The skill changed. Refresh before saving or deleting it.")
	ErrReadOnly           = errors.New("This linked skill cannot be modified here. Add a project-local copy to edit it.")
	ErrInvalidSource      = errors.New("Use nonempty UTF-8 skill content of at most 256 KiB with valid YAML front matter.")
	ErrInvalidName        = errors.New("Use a skill folder name of at most 64 lowercase letters, digits, and single hyphens.")
	ErrInvalidDisplayName = errors.New("Use a nonempty UTF-8 skill name of at most 128 characters without control characters.")
	skillName             = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
)

type managementPlan struct {
	edit     string
	remove   []string
	revision string
}

func sourceHash(raw []byte) string { sum := sha256.Sum256(raw); return hex.EncodeToString(sum[:]) }

// Only ordinary installations and leaf links can be removed. A shared-root or
// category link may contain other skills, so it cannot authorize a wider removal.
func planManagement(directory string, value definition) (managementPlan, error) {
	root, err := os.OpenRoot(directory)
	if err != nil {
		return managementPlan{}, err
	}
	defer root.Close()
	plan := managementPlan{}
	stamps := []string{value.path}
	for _, relative := range value.skill.Paths {
		base := ""
		for _, candidate := range roots {
			if relative == candidate+"/SKILL.md" || strings.HasPrefix(relative, candidate+"/") {
				base = candidate
				break
			}
		}
		if base == "" || path.Clean(relative) != relative || path.Base(relative) != "SKILL.md" {
			return managementPlan{}, ErrReadOnly
		}
		folder := path.Dir(relative)
		current, linked := "", false
		for _, part := range strings.Split(relative, "/") {
			current = path.Join(current, part)
			info, err := root.Lstat(current)
			if err != nil {
				return managementPlan{}, err
			}
			stamps = append(stamps, fmt.Sprintf("%s:%d:%d:%d", current, info.Mode(), info.Size(), info.ModTime().UnixNano()))
			if info.Mode()&fs.ModeSymlink != 0 {
				// A link to a single installed folder or definition can be detached.
				if current != folder && current != relative || current == base {
					return managementPlan{}, ErrReadOnly
				}
				plan.remove = append(plan.remove, current)
				linked = true
				break
			}
			if current != relative && !info.IsDir() || current == relative && !info.Mode().IsRegular() {
				return managementPlan{}, ErrReadOnly
			}
		}
		if !linked {
			if plan.edit == "" {
				plan.edit = relative
			}
			if folder == base {
				plan.remove = append(plan.remove, relative)
			} else {
				plan.remove = append(plan.remove, folder)
			}
		}
	}
	if raw, err := readFile(value.path); err == nil {
		stamps = append(stamps, sourceHash(raw))
	}
	sort.Strings(stamps)
	plan.revision = sourceHash([]byte(strings.Join(stamps, "\x00")))
	return plan, nil
}

func validateSource(content string) error {
	if strings.TrimSpace(content) == "" || len(content) > maxFileBytes || !utf8.ValidString(content) || strings.ContainsRune(content, 0) {
		return ErrInvalidSource
	}
	body := strings.TrimPrefix(content, "\ufeff")
	lines := strings.SplitAfter(body, "\n")
	if strings.TrimSpace(lines[0]) != "---" {
		return nil
	}
	end := len(lines[0])
	for _, line := range lines[1:] {
		if end > maxFrontMatterBytes {
			return ErrInvalidSource
		}
		if strings.TrimSpace(line) == "---" {
			var metadata struct {
				Name        string `yaml:"name"`
				Description string `yaml:"description"`
			}
			if yaml.Unmarshal([]byte(body[len(lines[0]):end]), &metadata) != nil {
				return ErrInvalidSource
			}
			return nil
		}
		end += len(line)
	}
	return ErrInvalidSource
}

func ordinaryParents(root *os.Root, relative string) error {
	current := ""
	for _, part := range strings.Split(path.Dir(relative), "/") {
		current = path.Join(current, part)
		info, err := root.Lstat(current)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&fs.ModeSymlink != 0 {
			return ErrReadOnly
		}
	}
	return nil
}

func findDefinition(ctx context.Context, directory, id string) (*scanner, definition, error) {
	s, err := scan(ctx, directory)
	if err != nil {
		return nil, definition{}, err
	}
	for _, value := range s.definitions {
		if value.skill.ID == id {
			return s, value, nil
		}
	}
	return s, definition{}, ErrNotFound
}

// New definitions are shared project-local skills. Existing files and folders
// are never replaced by creation; writes install a complete synced definition.
func Create(ctx context.Context, directory, name, content string) (Detail, error) {
	if len(name) > 64 || !skillName.MatchString(name) {
		return Detail{}, ErrInvalidName
	}
	if err := validateSource(content); err != nil {
		return Detail{}, err
	}
	if err := ctx.Err(); err != nil {
		return Detail{}, err
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return Detail{}, err
	}
	defer root.Close()
	folder := path.Join(".agents/skills", name)
	if err := ordinaryParents(root, folder+"/SKILL.md"); err != nil {
		return Detail{}, err
	}
	if err := root.MkdirAll(path.Dir(folder), 0755); err != nil {
		return Detail{}, err
	}
	if err := root.Mkdir(folder, 0755); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return Detail{}, ErrConflict
		}
		return Detail{}, err
	}
	installed := false
	defer func() {
		if !installed {
			root.Remove(folder)
		}
	}()
	if err := writeSource(root, folder+"/SKILL.md", content, ""); err != nil {
		return Detail{}, err
	}
	installed = true
	resolved, err := filepath.EvalSymlinks(filepath.Join(directory, filepath.FromSlash(folder), "SKILL.md"))
	if err != nil {
		return Detail{}, err
	}
	sum := sha256.Sum256([]byte(resolved))
	return Read(ctx, directory, hex.EncodeToString(sum[:16]))
}

func Update(ctx context.Context, directory, id, content, expectedHash string) (Detail, error) {
	if expectedHash == "" {
		return Detail{}, ErrConflict
	}
	if err := validateSource(content); err != nil {
		return Detail{}, err
	}
	s, value, err := findDefinition(ctx, directory, id)
	if err != nil {
		return Detail{}, err
	}
	plan, err := planManagement(s.root, value)
	if err != nil || plan.edit == "" {
		return Detail{}, ErrReadOnly
	}
	root, err := os.OpenRoot(s.root)
	if err != nil {
		return Detail{}, err
	}
	defer root.Close()
	if err := writeSource(root, plan.edit, content, expectedHash); err != nil {
		return Detail{}, err
	}
	return Read(ctx, directory, id)
}

func writeSource(root *os.Root, relative, content, expectedHash string) error {
	if err := ordinaryParents(root, relative); err != nil {
		return err
	}
	mode := fs.FileMode(0644)
	if expectedHash != "" {
		info, err := root.Lstat(relative)
		if err != nil || !info.Mode().IsRegular() || info.Size() > maxFileBytes {
			return ErrConflict
		}
		mode = info.Mode().Perm()
		raw, err := root.ReadFile(relative)
		if err != nil || sourceHash(raw) != expectedHash {
			return ErrConflict
		}
	}
	temporary := path.Join(path.Dir(relative), ".loom-"+rand.Text()+".tmp")
	file, err := root.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	defer root.Remove(temporary)
	if _, err = file.WriteString(content); err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if expectedHash == "" {
		if err := root.Link(temporary, relative); err != nil {
			if errors.Is(err, fs.ErrExist) {
				return ErrConflict
			}
			return err
		}
		return nil
	}
	if err := ordinaryParents(root, relative); err != nil {
		return err
	}
	raw, err := root.ReadFile(relative)
	if err != nil || sourceHash(raw) != expectedHash {
		return ErrConflict
	}
	return root.Rename(temporary, relative)
}

func Delete(ctx context.Context, directory, id, revision string) error {
	s, value, err := findDefinition(ctx, directory, id)
	if err != nil {
		return err
	}
	plan, err := planManagement(s.root, value)
	if err != nil || len(plan.remove) == 0 {
		return ErrReadOnly
	}
	if revision == "" || revision != plan.revision {
		return ErrConflict
	}
	root, err := os.OpenRoot(s.root)
	if err != nil {
		return err
	}
	defer root.Close()
	// Detach leaf aliases first; ordinary files/directories remain if that fails.
	linked := map[string]bool{}
	for _, relative := range plan.remove {
		info, err := root.Lstat(relative)
		if err != nil {
			return ErrConflict
		}
		linked[relative] = info.Mode()&fs.ModeSymlink != 0
	}
	sort.SliceStable(plan.remove, func(i, j int) bool {
		return linked[plan.remove[i]] && !linked[plan.remove[j]]
	})
	seen := map[string]bool{}
	for _, relative := range plan.remove {
		if seen[relative] {
			continue
		}
		seen[relative] = true
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := ordinaryParents(root, relative); err != nil {
			return err
		}
		if err := root.RemoveAll(relative); err != nil {
			return err
		}
	}
	return nil
}
