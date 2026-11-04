package credentials

import (
	"errors"
	"os"
	"path/filepath"
)

type Store struct{ directory string }

func New(directory string) (*Store, error) {
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}
	return &Store{directory: directory}, nil
}

func (s *Store) Save(id, token string) error {
	data, err := protect([]byte(token))
	if err != nil {
		return err
	}
	file, err := os.OpenFile(filepath.Join(s.directory, id+".secret.tmp"), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), filepath.Join(s.directory, id+".secret"))
}

func (s *Store) Read(id string) (string, error) {
	data, err := os.ReadFile(filepath.Join(s.directory, id+".secret"))
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", errors.New("cannot read the saved service credential")
	}
	plain, err := unprotect(data)
	if err != nil {
		return "", errors.New("cannot decrypt the service credential for this user")
	}
	return string(plain), nil
}
