package credentials

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

func TestVaultEncryptionSurvivesRestartAndAuthenticatesRecords(t *testing.T) {
	directory := t.TempDir()
	vault, err := NewVault(directory)
	if err != nil {
		t.Fatal(err)
	}
	plain := []byte(`{"token":"dummy-platform-token"}`)
	sealed, err := vault.Seal("first", plain)
	if err != nil {
		t.Fatal(err)
	}
	second, err := vault.Seal("first", plain)
	if err != nil || bytes.Equal(sealed, second) || bytes.Contains(sealed, plain) {
		t.Fatal("ciphertext was not randomized and encrypted")
	}
	restarted, err := NewVault(directory)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := restarted.Open("first", sealed)
	if err != nil || !bytes.Equal(decoded, plain) {
		t.Fatal("credential did not survive restart")
	}
	if _, err := restarted.Open("second", sealed); err == nil {
		t.Fatal("ciphertext was accepted for another record")
	}
	tampered := bytes.Clone(sealed)
	tampered[len(tampered)-1] ^= 1
	for _, value := range [][]byte{nil, sealed[:10], append([]byte{2}, sealed[1:]...), tampered} {
		if _, err := restarted.Open("first", value); err == nil {
			t.Fatal("corrupt ciphertext was accepted")
		}
	}
}

func TestConcurrentVaultCreationUsesOneCompleteKey(t *testing.T) {
	directory := t.TempDir()
	const count = 12
	var workers sync.WaitGroup
	results := make(chan *Vault, count)
	errors := make(chan error, count)
	for range count {
		workers.Add(1)
		go func() {
			defer workers.Done()
			vault, err := NewVault(directory)
			if err != nil {
				errors <- err
			} else {
				results <- vault
			}
		}()
	}
	workers.Wait()
	close(results)
	close(errors)
	for err := range errors {
		t.Fatal(err)
	}
	var ciphertext []byte
	for vault := range results {
		if ciphertext == nil {
			ciphertext, _ = vault.Seal("id", []byte("test-token"))
		}
		if plain, err := vault.Open("id", ciphertext); err != nil || string(plain) != "test-token" {
			t.Fatal("concurrent startup used different keys")
		}
	}
	files, _ := filepath.Glob(filepath.Join(directory, ".vault-key-*"))
	if len(files) != 0 {
		t.Fatal("temporary key files remained")
	}
}

func TestVaultRejectsInvalidKeysAndRepairsPermissions(t *testing.T) {
	directory := t.TempDir()
	if _, err := NewVault(directory); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "vault.key")
	if runtime.GOOS != "windows" {
		os.Chmod(directory, 0755)
		os.Chmod(path, 0644)
		if _, err := NewVault(directory); err != nil {
			t.Fatal(err)
		}
		for name, want := range map[string]os.FileMode{directory: 0700, path: 0600} {
			info, err := os.Stat(name)
			if err != nil || info.Mode().Perm() != want {
				t.Fatal("vault permissions were not owner-only")
			}
		}
	}
	if err := os.WriteFile(path, []byte("invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewVault(directory); err == nil {
		t.Fatal("invalid key was accepted")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := NewVault(directory); err == nil {
		t.Fatal("non-regular key was accepted")
	}
}
