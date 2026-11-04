//go:build !windows

package credentials

// Unix credentials are protected by owner-only file and directory permissions.
func protect(data []byte) ([]byte, error)   { return data, nil }
func unprotect(data []byte) ([]byte, error) { return data, nil }
