package materials

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"
)

// Workflow files are private, bounded and confined like ordinary materials.
func workflowPath(scope Scope, relative string) (string, error) {
	if relative == "" || path.Clean(relative) != relative || strings.ContainsAny(relative, "\\\x00") || strings.HasPrefix(relative, "/") || strings.HasPrefix(relative, "../") {
		return "", errors.New("Invalid workflow file path.")
	}
	return path.Join(scope.Root, ".workflow", relative), nil
}

func ReadWorkflow(scope Scope, relative string) ([]byte, error) {
	name, err := workflowPath(scope, relative)
	if err != nil {
		return nil, err
	}
	root, err := openRoot(scope)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	if err := plainDirectories(root, path.Dir(name)); err != nil {
		return nil, err
	}
	info, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("Workflow files must be ordinary files.")
	}
	file, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	value, err := io.ReadAll(io.LimitReader(file, MaxEditBytes+1))
	if len(value) > MaxEditBytes {
		return nil, errors.New("Workflow files must be at most 1 MiB.")
	}
	return value, err
}

// WriteWorkflow creates immutable snapshots, never replaces an existing file.
func WriteWorkflow(scope Scope, relative string, value []byte) error {
	if len(value) > MaxEditBytes {
		return errors.New("Workflow files must be at most 1 MiB.")
	}
	name, err := workflowPath(scope, relative)
	if err != nil {
		return err
	}
	root, err := openRoot(scope)
	if err != nil {
		return err
	}
	defer root.Close()
	if err := plainDirectories(root, path.Dir(name)); err != nil {
		return err
	}
	if err := root.MkdirAll(path.Dir(name), 0755); err != nil {
		return err
	}
	if existing, err := ReadWorkflow(scope, relative); err == nil {
		if string(existing) == string(value) {
			return nil
		}
		return ErrConflict
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	var nonce [12]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return err
	}
	temporary := path.Join(path.Dir(name), ".snapshot-"+hex.EncodeToString(nonce[:]))
	file, err := root.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer root.Remove(temporary)
	_, writeErr := file.Write(value)
	if writeErr == nil {
		writeErr = file.Sync()
	}
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	return root.Link(temporary, name)
}
