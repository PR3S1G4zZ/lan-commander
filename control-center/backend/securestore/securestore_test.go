package securestore

import (
	"errors"
	"runtime"
	"testing"
)

func TestDefaultStoreProtectsAndRestoresTokens(t *testing.T) {
	store := Default()
	protected, err := store.Protect("token-value")
	if runtime.GOOS != "windows" {
		if !errors.Is(err, ErrUnavailable) {
			t.Fatalf("Protect error = %v, want ErrUnavailable outside Windows", err)
		}
		return
	}
	if err != nil {
		t.Fatalf("protect token: %v", err)
	}
	if protected == "token-value" {
		t.Fatal("protected token was returned as plaintext")
	}
	restored, err := store.Unprotect(protected)
	if err != nil {
		t.Fatalf("unprotect token: %v", err)
	}
	if restored != "token-value" {
		t.Fatalf("restored token = %q, want token-value", restored)
	}
}

func TestUnavailableStoreFailsClosed(t *testing.T) {
	store := unavailableStore{}
	if _, err := store.Protect("token-value"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Protect error = %v, want ErrUnavailable", err)
	}
	if _, err := store.Unprotect("ciphertext"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Unprotect error = %v, want ErrUnavailable", err)
	}
}
