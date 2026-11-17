package materials

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
)

func TestWorkflowSnapshotsAreImmutableConfinedAndNotIndexed(t *testing.T) {
	directory := t.TempDir()
	scope := NewScope("project", "requirement", directory, "main", "commit")
	if err := WriteWorkflow(scope, "flow/approved/doc.md", []byte("Approved snapshot")); err != nil {
		t.Fatal(err)
	}
	if err := WriteWorkflow(scope, "flow/approved/doc.md", []byte("Changed")); err != ErrConflict {
		t.Fatal("snapshot was mutable", err)
	}
	for _, relative := range []string{"../outside", "/absolute", "flow/../../outside", "flow\\escape"} {
		if _, err := ReadWorkflow(scope, relative); err == nil {
			t.Fatal("read escaped private root")
		}
	}
	t.Run("rejects symlinks", func(t *testing.T) {
		if err := os.Symlink(directory, filepath.Join(directory, scope.Root, ".workflow", "link")); err != nil {
			if runtime.GOOS == "windows" && errors.Is(err, syscall.Errno(1314)) {
				t.Skipf("symlink privilege unavailable: %v", err)
			}
			t.Fatal(err)
		}
		if err := WriteWorkflow(scope, "link/escape", []byte("Escape")); err == nil {
			t.Fatal("write followed symlink")
		}
		if err := os.Symlink(filepath.Join(directory, scope.Root, ".workflow", "flow/approved/doc.md"), filepath.Join(directory, scope.Root, ".workflow", "symlink.md")); err != nil {
			t.Fatal(err)
		}
		if _, err := ReadWorkflow(scope, "symlink.md"); err == nil {
			t.Fatal("read followed symlink")
		}
	})
	catalog, err := Scan(t.Context(), scope)
	if err != nil || len(catalog.Files) != 0 {
		t.Fatal("private snapshots appeared as user materials", err)
	}
}
