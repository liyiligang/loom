package skills

import (
	"context"
	"path"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/parser"
)

// Display names are user text; identifiers remain compatible with skill loaders.
func validDisplayName(name string) bool {
	if name == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > 128 {
		return false
	}
	for _, character := range name {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func installationName(displayName string) string {
	if len(displayName) <= 64 && skillName.MatchString(displayName) {
		return displayName
	}
	return "skill-" + sourceHash([]byte(displayName))[:32]
}

type sourceParts struct{ prefix, metadata, closing, body string }

func splitSource(content string) sourceParts {
	bom := ""
	if strings.HasPrefix(content, "\ufeff") {
		bom, content = "\ufeff", strings.TrimPrefix(content, "\ufeff")
	}
	lines := strings.SplitAfter(content, "\n")
	if strings.TrimSpace(lines[0]) == "---" {
		end := len(lines[0])
		for _, line := range lines[1:] {
			if strings.TrimSpace(line) == "---" {
				return sourceParts{bom + lines[0], content[len(lines[0]):end], line, content[end+len(line):]}
			}
			end += len(line)
		}
	}
	return sourceParts{prefix: bom + "---\n", closing: "---\n\n", body: content}
}

func nativeName(content, fallback string) string {
	var metadata struct {
		Name string `yaml:"name"`
	}
	if yaml.Unmarshal([]byte(splitSource(content).metadata), &metadata) == nil && len(metadata.Name) <= 64 && skillName.MatchString(metadata.Name) {
		return metadata.Name
	}
	return fallback
}

func namedSource(content, displayName, identifier string) (string, error) {
	displayName = strings.TrimSpace(displayName)
	if !validDisplayName(displayName) {
		return "", ErrInvalidDisplayName
	}
	if err := validateSource(content); err != nil {
		return "", err
	}
	parts := splitSource(content)
	var metadata struct {
		Name     string         `yaml:"name"`
		Metadata map[string]any `yaml:"metadata"`
	}
	if yaml.Unmarshal([]byte(parts.metadata), &metadata) != nil {
		return "", ErrInvalidSource
	}
	display, _ := metadata.Metadata["display_name"].(string)
	if metadata.Name == identifier && (display == displayName || display == "" && displayName == identifier) {
		return content, nil
	}
	header := parts.metadata
	if strings.TrimSpace(header) == "" {
		header = "name: " + strconv.Quote(identifier) + "\n"
	}
	file, err := parser.ParseBytes([]byte(header), parser.ParseComments)
	if err != nil {
		return "", ErrInvalidSource
	}
	root, _ := yaml.PathString("$")
	if metadata.Name != identifier {
		if err := root.MergeFromReader(file, strings.NewReader("name: "+strconv.Quote(identifier)+"\n")); err != nil {
			return "", ErrInvalidSource
		}
	}
	if displayName != identifier || display != "" {
		field := "display_name: " + strconv.Quote(displayName) + "\n"
		if metadata.Metadata != nil {
			location, _ := yaml.PathString("$.metadata")
			err = location.MergeFromReader(file, strings.NewReader(field))
		} else {
			err = root.MergeFromReader(file, strings.NewReader("metadata:\n  "+field))
		}
		if err != nil {
			return "", ErrInvalidSource
		}
	}
	header = strings.TrimRight(file.String(), "\n") + "\n"
	if strings.HasSuffix(parts.prefix, "\r\n") {
		header = strings.ReplaceAll(header, "\n", "\r\n")
	}
	result := parts.prefix + header + parts.closing + parts.body
	if err := validateSource(result); err != nil {
		return "", err
	}
	return result, nil
}

func CreateNamed(ctx context.Context, directory, displayName, content string) (Detail, error) {
	displayName = strings.TrimSpace(displayName)
	identifier := installationName(displayName)
	source, err := namedSource(content, displayName, identifier)
	if err != nil {
		return Detail{}, err
	}
	return Create(ctx, directory, identifier, source)
}

func UpdateNamed(ctx context.Context, directory, id, displayName, content, expectedHash string) (Detail, error) {
	existing, err := Read(ctx, directory, id)
	if err != nil {
		return Detail{}, err
	}
	fallback := path.Base(path.Dir(existing.Paths[0]))
	if len(fallback) > 64 || !skillName.MatchString(fallback) {
		fallback = "skill-" + id
	}
	identifier := nativeName(content, nativeName(existing.Source, fallback))
	source, err := namedSource(content, displayName, identifier)
	if err != nil {
		return Detail{}, err
	}
	return Update(ctx, directory, id, source, expectedHash)
}
