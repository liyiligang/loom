package materials

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestFilesAreAuthoritativeAndWritesPreserveExternalChanges(t *testing.T) {
	scope := NewScope("p", "r", t.TempDir(), "main", "first")
	catalog, err := Scan(context.Background(), scope)
	if err != nil || len(catalog.Files) != 0 {
		t.Fatalf("empty scan: %+v, %v", catalog, err)
	}
	if _, err := os.Stat(filepath.Join(scope.Directory, scope.Root)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("read created material directories")
	}
	for _, value := range []struct{ kind, name, content string }{
		{"documents", "requirements.md", "# Requirement\n"}, {"decisions", "choice.md", "# Decision\n"},
		{"testCases", "cases.yaml", "cases:\n  - name: Sign in\n"}, {"datasets", "inputs.csv", "name,expected\nalice,ok\n"},
		{"loopDesign", "loop.md", "# Loop\n"}, {"designDocuments", "design.md", "# Design\n"},
		{"prototypes", "demo.html", "<!doctype html><title>Demo</title><button>Preview</button><script>document.title = 'Interactive demo'</script>"},
	} {
		id, err := Write(scope, value.kind, value.name, value.content, "")
		if err != nil {
			t.Fatal(err)
		}
		catalog, err = Scan(context.Background(), scope)
		if err != nil {
			t.Fatal(err)
		}
		file, err := Find(catalog, id)
		if err != nil {
			t.Fatal(err)
		}
		detail, err := Read(scope, file)
		if err != nil || detail.Content != value.content || file.Hash == "" {
			t.Fatalf("file round trip: %+v, %v", detail, err)
		}
	}
	file := catalog.Files[0]
	if err := os.WriteFile(filepath.Join(scope.Directory, file.Path), []byte("External edit\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(scope, file); err == nil {
		t.Fatal("stale read was accepted")
	}
	if _, err := Write(scope, file.Kind, filepath.Base(file.Path), "Stale browser edit", file.Hash); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale save: %v", err)
	}
	raw, _ := os.ReadFile(filepath.Join(scope.Directory, file.Path))
	if string(raw) != "External edit\n" {
		t.Fatal("external content was overwritten")
	}
	if _, err := Write(scope, file.Kind, filepath.Base(file.Path), "Duplicate", ""); !errors.Is(err, ErrConflict) {
		t.Fatal("creation replaced an existing file")
	}
	catalog, err = Scan(context.Background(), scope)
	if err != nil {
		t.Fatal(err)
	}
	file = catalog.Files[0]
	if _, err := Write(scope, file.Kind, filepath.Base(file.Path), "Updated\n", file.Hash); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(filepath.Dir(filepath.Join(scope.Directory, file.Path)))
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".loom-") {
			t.Fatal("temporary file was retained")
		}
	}
}

func TestExclusiveCreationAndFormatValidation(t *testing.T) {
	scope := NewScope("p", "r", t.TempDir(), "", "")
	var wg sync.WaitGroup
	errorsFound := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := Write(scope, "documents", "one.md", "# One", "")
			errorsFound <- err
		}()
	}
	wg.Wait()
	close(errorsFound)
	successes, conflicts := 0, 0
	for err := range errorsFound {
		if err == nil {
			successes++
		} else if errors.Is(err, ErrConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatal("concurrent creation did not preserve one complete file")
	}
	for _, value := range []struct{ kind, name, content string }{
		{"documents", "../escape.md", "text"}, {"documents", "..\\escape.md", "text"}, {"unknown", "a.md", "text"},
		{"testCases", "a.json", "{"}, {"testCases", "a.yaml", "a: ["}, {"datasets", "a.csv", "a,b\n1\n"},
		{"datasets", "a.jsonl", "{}\nnot-json"}, {"datasets", "a.parquet", "text"},
		{"documents", "large.md", strings.Repeat("x", MaxEditBytes+1)}, {"documents", "invalid.md", "\xff"},
		{"prototypes", "demo.js", "alert(1)"}, {"prototypes", "../demo.html", "<h1>Demo</h1>"}, {"documents", "demo.html", "<h1>Demo</h1>"},
	} {
		if Validate(value.kind, value.name, value.content) == nil {
			t.Fatalf("accepted invalid material: %s/%s", value.kind, value.name)
		}
	}
}

func TestScanAndAccessRejectSymlinksAndUnsafePaths(t *testing.T) {
	scope := NewScope("p", "r", t.TempDir(), "main", "c")
	_, err := Write(scope, "documents", "safe.md", "Safe", "")
	if err != nil {
		t.Fatal(err)
	}
	external := t.TempDir()
	if err := os.WriteFile(filepath.Join(external, "outside.md"), []byte("Outside"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(external, "outside.md"), filepath.Join(scope.Directory, scope.Root, "documents", "link.md")); err != nil {
		t.Skip(err)
	}
	if err := os.Symlink(external, filepath.Join(scope.Directory, scope.Root, "decisions")); err != nil {
		t.Fatal(err)
	}
	catalog, err := Scan(context.Background(), scope)
	if err != nil || len(catalog.Files) != 1 {
		t.Fatalf("scan followed a symlink: %+v %v", catalog, err)
	}
	if _, err := Write(scope, "decisions", "outside.md", "Overwrite", ""); err == nil {
		t.Fatal("writer followed symlinked directory")
	}
	for _, file := range []File{{Kind: "documents", Path: "../outside.md"}, {Kind: "documents", Path: scope.Root + "/documents/link.md"}} {
		if stream, err := Open(scope, file); err == nil {
			stream.Close()
			t.Fatal("download followed unsafe file")
		}
	}
	parentScope := NewScope("p", "other", t.TempDir(), "", "")
	if err := os.Symlink(external, filepath.Join(parentScope.Directory, "docs")); err != nil {
		t.Fatal(err)
	}
	if _, err := Scan(context.Background(), parentScope); err == nil {
		t.Fatal("scan followed symlinked ancestor")
	}
	if _, err := Write(parentScope, "documents", "a.md", "Text", ""); err == nil {
		t.Fatal("write followed symlinked ancestor")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Scan(ctx, scope); !errors.Is(err, context.Canceled) {
		t.Fatalf("scan ignored cancellation: %v", err)
	}
}

func TestDatasetPreviewBoundsAndBranchIdentity(t *testing.T) {
	scope := NewScope("p", "r", t.TempDir(), "main", "one")
	if scope.Key != NewScope("p", "r", scope.Directory, "main", "two").Key || scope.Key == NewScope("p", "r", scope.Directory, "other", "one").Key {
		t.Fatal("branch index scope is unstable or shared")
	}
	if NewScope("p", "r", scope.Directory, "", "one").Key == NewScope("p", "r", scope.Directory, "", "two").Key {
		t.Fatal("detached commit scope is shared")
	}
	directory := filepath.Join(scope.Directory, scope.Root, "datasets")
	if err := os.MkdirAll(directory, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "large.jsonl"), []byte(strings.Repeat("{\"v\":\"é\"}\n", MaxEditBytes/8)), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "binary.parquet"), []byte("PAR1\x00"), 0644); err != nil {
		t.Fatal(err)
	}
	catalog, err := Scan(context.Background(), scope)
	if err != nil || len(catalog.Files) != 2 {
		t.Fatalf("dataset scan: %+v %v", catalog, err)
	}
	for _, file := range catalog.Files {
		detail, err := Read(scope, file)
		if err != nil || detail.Editable || len(detail.Content) > MaxEditBytes {
			t.Fatalf("dataset read: %+v %v", detail.File, err)
		}
		if strings.HasSuffix(file.Path, ".jsonl") && !detail.Truncated {
			t.Fatal("large preview was not marked incomplete")
		}
		stream, err := Open(scope, file)
		if err != nil {
			t.Fatal(err)
		}
		stream.Close()
	}
}
