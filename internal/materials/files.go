// Package materials owns requirement files; database records are rebuildable indexes.
package materials

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/goccy/go-yaml"
)

const MaxEditBytes = 1 << 20
const maxHashBytes = 64 << 20
const maxFiles = 512

var Folders = map[string]string{"documents": "documents", "decisions": "decisions", "testCases": "test-cases", "datasets": "datasets", "loopDesign": "loop-design", "designDocuments": "design-documents", "prototypes": "prototypes"}

type Scope struct {
	ProjectID     string `json:"projectId"`
	RequirementID string `json:"requirementId"`
	Directory     string `json:"directory"`
	Branch        string `json:"branch"`
	Commit        string `json:"commit"`
	Key           string `json:"key"`
	Root          string `json:"root"`
}
type File struct {
	ID         string   `json:"id"`
	Kind       string   `json:"kind"`
	Path       string   `json:"path"`
	Title      string   `json:"title"`
	Hash       string   `json:"hash"`
	Size       int64    `json:"size"`
	ModifiedAt string   `json:"modifiedAt"`
	Editable   bool     `json:"editable"`
	RelatedIDs []string `json:"relatedIds"`
}
type Catalog struct {
	Scope   Scope  `json:"scope"`
	Files   []File `json:"files"`
	Limited bool   `json:"limited"`
}
type Detail struct {
	File
	Scope     Scope  `json:"scope"`
	Content   string `json:"content"`
	Truncated bool   `json:"truncated"`
}

func digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
func NewScope(projectID, requirementID, directory, branch, commit string) Scope {
	ref := branch
	if ref == "" {
		ref = "HEAD:" + commit
	}
	return Scope{ProjectID: projectID, RequirementID: requirementID, Directory: directory, Branch: branch, Commit: commit, Key: digest(directory + "\x00" + ref), Root: path.Join("docs/requirements", requirementID)}
}
func FileID(scope Scope, relative string) string {
	return digest(scope.RequirementID + "\x00" + relative)[:32]
}
func formatAllowed(kind, name string) bool {
	ext := strings.ToLower(path.Ext(name))
	switch kind {
	case "datasets":
		return ext == ".csv" || ext == ".jsonl" || ext == ".json" || ext == ".parquet"
	case "testCases":
		return ext == ".json" || ext == ".yaml" || ext == ".yml"
	case "loopDesign":
		return ext == ".md" || ext == ".json"
	case "prototypes":
		return ext == ".html" || ext == ".htm"
	default:
		_, ok := Folders[kind]
		return ok && ext == ".md"
	}
}
func ValidName(kind, name string) bool {
	return name != "" && len(name) <= 128 && path.Base(name) == name && !strings.ContainsAny(name, "\\\x00\r\n") && !strings.HasPrefix(name, ".") && formatAllowed(kind, name)
}
func openRoot(scope Scope) (*os.Root, error) {
	if scope.RequirementID == "" || strings.ContainsAny(scope.RequirementID, "/\\\x00") || scope.RequirementID == "." || scope.RequirementID == ".." {
		return nil, errors.New("Invalid requirement file scope.")
	}
	if scope.Root != path.Join("docs/requirements", scope.RequirementID) {
		return nil, errors.New("Invalid requirement file root.")
	}
	root, err := os.OpenRoot(scope.Directory)
	if err != nil {
		return nil, err
	}
	if err := plainDirectories(root, scope.Root); err != nil {
		root.Close()
		return nil, err
	}
	return root, nil
}

func plainDirectories(root *os.Root, directory string) error {
	current := ""
	for _, part := range strings.Split(directory, "/") {
		current = path.Join(current, part)
		info, err := root.Lstat(current)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&fs.ModeSymlink != 0 {
			return errors.New("Requirement file directories must be ordinary directories, not symlinks.")
		}
	}
	return nil
}

func openMaterial(root *os.Root, scope Scope, file File) (*os.File, error) {
	name := path.Base(file.Path)
	if !ValidName(file.Kind, name) || file.Path != path.Join(scope.Root, Folders[file.Kind], name) {
		return nil, errors.New("Invalid requirement file path.")
	}
	if err := plainDirectories(root, path.Dir(file.Path)); err != nil {
		return nil, err
	}
	info, err := root.Lstat(file.Path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("Requirement files must be ordinary files, not symlinks.")
	}
	return root.Open(file.Path)
}

// Scan never follows symlinked material files or directories. OpenRoot confines every read.
func Scan(ctx context.Context, scope Scope) (Catalog, error) {
	result := Catalog{Scope: scope, Files: []File{}}
	root, err := openRoot(scope)
	if err != nil {
		return result, err
	}
	defer root.Close()
	if _, err := root.Lstat(scope.Root); errors.Is(err, fs.ErrNotExist) {
		return result, nil
	} else if err != nil {
		return result, err
	}
	count := 0
	err = fs.WalkDir(root.FS(), scope.Root, func(relative string, entry fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return walkErr
		}
		count++
		if count > 4096 || len(result.Files) >= maxFiles {
			result.Limited = true
			return fs.SkipAll
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		if entry.IsDir() {
			if relative != scope.Root && strings.Contains(strings.TrimPrefix(relative, scope.Root+"/"), "/") {
				return fs.SkipDir
			}
			if relative != scope.Root && strings.HasPrefix(entry.Name(), ".") {
				return fs.SkipDir
			}
			return nil
		}
		parts := strings.Split(strings.TrimPrefix(relative, scope.Root+"/"), "/")
		if len(parts) != 2 {
			return nil
		}
		kind := ""
		for value, folder := range Folders {
			if folder == parts[0] {
				kind = value
			}
		}
		if !ValidName(kind, parts[1]) {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		file := File{ID: FileID(scope, relative), Kind: kind, Path: relative, Title: strings.TrimSuffix(parts[1], path.Ext(parts[1])), Size: info.Size(), ModifiedAt: info.ModTime().UTC().Format("2006-01-02T15:04:05.999999999Z"), Editable: info.Size() <= MaxEditBytes && strings.ToLower(path.Ext(parts[1])) != ".parquet", RelatedIDs: []string{}}
		if info.Size() <= maxHashBytes {
			stream, err := openMaterial(root, scope, file)
			if err != nil {
				return err
			}
			hash := sha256.New()
			buffer := make([]byte, 64<<10)
			read := int64(0)
			for {
				if ctx.Err() != nil {
					stream.Close()
					return ctx.Err()
				}
				n, readErr := stream.Read(buffer)
				read += int64(n)
				if read > maxHashBytes {
					stream.Close()
					return errors.New("A material changed size while being indexed. Refresh and retry.")
				}
				hash.Write(buffer[:n])
				if readErr == io.EOF {
					break
				}
				if readErr != nil {
					stream.Close()
					return readErr
				}
			}
			stream.Close()
			file.Hash = hex.EncodeToString(hash.Sum(nil))
		}
		result.Files = append(result.Files, file)
		return nil
	})
	sort.Slice(result.Files, func(i, j int) bool { return result.Files[i].Path < result.Files[j].Path })
	return result, err
}
func Find(catalog Catalog, id string) (File, error) {
	for _, file := range catalog.Files {
		if file.ID == id {
			return file, nil
		}
	}
	return File{}, fs.ErrNotExist
}
func Read(scope Scope, file File) (Detail, error) {
	result := Detail{File: file, Scope: scope}
	if strings.ToLower(path.Ext(file.Path)) == ".parquet" {
		return result, nil
	}
	root, err := openRoot(scope)
	if err != nil {
		return result, err
	}
	defer root.Close()
	stream, err := openMaterial(root, scope, file)
	if err != nil {
		return result, err
	}
	defer stream.Close()
	value, err := io.ReadAll(io.LimitReader(stream, MaxEditBytes+1))
	if err != nil {
		return result, err
	}
	if len(value) > MaxEditBytes {
		value = value[:MaxEditBytes]
		result.Truncated = true
		result.Editable = false
		for cut := 0; cut < utf8.UTFMax && !utf8.Valid(value) && len(value) > 0; cut++ {
			value = value[:len(value)-1]
		}
	}
	if !utf8.Valid(value) || strings.ContainsRune(string(value), 0) {
		return result, errors.New("Material files must contain UTF-8 text.")
	}
	result.Content = string(value)
	if !result.Truncated && file.Hash != digest(result.Content) {
		return result, errors.New("The material changed while it was being read. Refresh and retry.")
	}
	return result, nil
}
func Validate(kind, name, content string) error {
	if !ValidName(kind, name) || len(content) > MaxEditBytes || !utf8.ValidString(content) || strings.ContainsRune(content, 0) || strings.EqualFold(path.Ext(name), ".parquet") {
		return errors.New("Use an allowed filename and UTF-8 text of at most 1 MiB.")
	}
	switch strings.ToLower(path.Ext(name)) {
	case ".json":
		if !json.Valid([]byte(content)) {
			return errors.New("The material must contain valid JSON.")
		}
	case ".yaml", ".yml":
		var value any
		if yaml.Unmarshal([]byte(content), &value) != nil {
			return errors.New("The material must contain valid YAML.")
		}
	case ".jsonl":
		for _, line := range strings.Split(content, "\n") {
			if strings.TrimSpace(line) != "" && !json.Valid([]byte(line)) {
				return errors.New("Each dataset line must contain valid JSON.")
			}
		}
	case ".csv":
		reader := csv.NewReader(strings.NewReader(content))
		for {
			_, err := reader.Read()
			if err == io.EOF {
				break
			}
			if err != nil {
				return errors.New("The dataset must contain valid CSV with consistent columns.")
			}
		}
	}
	return nil
}

// Write commits a complete file before its derived index is refreshed. Existing files
// require an exact content hash; an empty expected hash means create exclusively.
func Write(scope Scope, kind, name, content, expectedHash string) (string, error) {
	if err := Validate(kind, name, content); err != nil {
		return "", err
	}
	root, err := openRoot(scope)
	if err != nil {
		return "", err
	}
	defer root.Close()
	directory := path.Join(scope.Root, Folders[kind])
	relative := path.Join(directory, name)
	if err := plainDirectories(root, directory); err != nil {
		return "", err
	}
	if err := root.MkdirAll(directory, 0755); err != nil {
		return "", err
	}
	if info, err := root.Lstat(relative); err == nil {
		if !info.Mode().IsRegular() || expectedHash == "" || info.Size() > MaxEditBytes {
			return "", ErrConflict
		}
		value, err := root.ReadFile(relative)
		if err != nil {
			return "", err
		}
		if digest(string(value)) != expectedHash {
			return "", ErrConflict
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", err
	} else if expectedHash != "" {
		return "", ErrConflict
	}
	temporary := path.Join(directory, ".loom-"+rand.Text()+".tmp")
	file, err := root.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return "", err
	}
	defer root.Remove(temporary)
	if _, err = file.WriteString(content); err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return "", err
	}
	if closeErr != nil {
		return "", closeErr
	}
	// Recheck immediately before committing to preserve intervening external edits.
	if expectedHash == "" {
		if err := root.Link(temporary, relative); err != nil {
			if errors.Is(err, fs.ErrExist) {
				return "", ErrConflict
			}
			return "", err
		}
	} else {
		value, err := root.ReadFile(relative)
		if err != nil || digest(string(value)) != expectedHash {
			return "", ErrConflict
		}
		if err := root.Rename(temporary, relative); err != nil {
			return "", err
		}
	}
	return FileID(scope, relative), nil
}

var ErrConflict = errors.New("The material changed or already exists. Refresh before saving.")

func Open(scope Scope, file File) (*os.File, error) {
	root, err := openRoot(scope)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	file.Path = filepath.ToSlash(file.Path)
	return openMaterial(root, scope, file)
}
func ErrorMessage(err error) string {
	return fmt.Sprintf("Could not access requirement files: %v", err)
}
