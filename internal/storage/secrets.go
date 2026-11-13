package storage

func (s *Store) SecretCiphertext(id string) ([]byte, error) {
	var sealed []byte
	err := s.db.QueryRow("SELECT ciphertext FROM secrets WHERE id = ?", id).Scan(&sealed)
	return sealed, err
}
