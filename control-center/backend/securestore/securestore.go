package securestore

import "errors"

// ErrUnavailable indicates that the platform does not provide the required
// protected token storage.
var ErrUnavailable = errors.New("secure token storage is unavailable on this platform")

// Store protects and restores sensitive session tokens.
type Store interface {
	Protect(plaintext string) (string, error)
	Unprotect(ciphertext string) (string, error)
}

type unavailableStore struct{}

func (unavailableStore) Protect(string) (string, error) {
	return "", ErrUnavailable
}

func (unavailableStore) Unprotect(string) (string, error) {
	return "", ErrUnavailable
}

// Default returns the platform-native protected token store.
func Default() Store {
	return defaultStore()
}
