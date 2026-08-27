//go:build !windows

package securestore

func defaultStore() Store {
	return unavailableStore{}
}
