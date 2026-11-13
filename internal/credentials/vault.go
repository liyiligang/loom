package credentials

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"runtime"
)

// Vault encrypts third-party credentials with a separate local master key.
type Vault struct{ cipher cipher.AEAD }

type Values struct {
	Content         string `json:"content,omitempty"`
	Token           string `json:"token,omitempty"`
	AccessKeyID     string `json:"accessKeyId,omitempty"`
	SecretAccessKey string `json:"secretAccessKey,omitempty"`
}

func NewVault(directory string) (*Vault, error) {
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(directory, 0700); err != nil {
			return nil, err
		}
	}
	path := filepath.Join(directory, "vault.key")
	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		key := make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			return nil, err
		}
		defer clear(key)
		protected, err := protect(key)
		if err != nil {
			return nil, err
		}
		file, err := os.CreateTemp(directory, ".vault-key-*")
		if err != nil {
			return nil, err
		}
		defer os.Remove(file.Name())
		if _, err := file.Write(protected); err != nil {
			file.Close()
			return nil, err
		}
		if err := file.Sync(); err != nil {
			file.Close()
			return nil, err
		}
		if err := file.Close(); err != nil {
			return nil, err
		}
		// Linking a complete file preserves the winner during concurrent startup.
		if err := os.Link(file.Name(), path); err != nil && !errors.Is(err, os.ErrExist) {
			return nil, err
		}
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 65536 {
		return nil, errors.New("Credential storage key is unavailable.")
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(path, 0600); err != nil {
			return nil, errors.New("Credential storage key is unavailable.")
		}
	}
	protected, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.New("Credential storage key is unavailable.")
	}
	defer clear(protected)
	key, err := unprotect(protected)
	if err != nil || len(key) != 32 {
		return nil, errors.New("Credential storage key is invalid.")
	}
	block, err := aes.NewCipher(key)
	clear(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	return &Vault{cipher: aead}, err
}

func (v *Vault) Seal(id string, plain []byte) ([]byte, error) {
	nonce := make([]byte, v.cipher.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return v.cipher.Seal(append([]byte{1}, nonce...), nonce, plain, []byte("loom-vault-v1:"+id)), nil
}

func (v *Vault) Open(id string, sealed []byte) ([]byte, error) {
	size := v.cipher.NonceSize()
	if len(sealed) < 1+size+v.cipher.Overhead() || sealed[0] != 1 {
		return nil, errors.New("Could not decrypt the saved credential.")
	}
	plain, err := v.cipher.Open(nil, sealed[1:1+size], sealed[1+size:], []byte("loom-vault-v1:"+id))
	if err != nil {
		return nil, errors.New("Could not decrypt the saved credential.")
	}
	return plain, nil
}
