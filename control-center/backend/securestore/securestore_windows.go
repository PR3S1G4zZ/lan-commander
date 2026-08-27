//go:build windows

package securestore

import (
	"encoding/base64"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

type dpapiStore struct{}

func defaultStore() Store {
	return dpapiStore{}
}

func (dpapiStore) Protect(plaintext string) (string, error) {
	input := []byte(plaintext)
	inBlob := windows.DataBlob{Size: uint32(len(input))}
	if len(input) > 0 {
		inBlob.Data = &input[0]
	}

	var outBlob windows.DataBlob
	if err := windows.CryptProtectData(&inBlob, nil, nil, 0, nil, 0, &outBlob); err != nil {
		return "", fmt.Errorf("DPAPI protect failed: %w", err)
	}
	defer freeDataBlob(&outBlob)

	protected := append([]byte(nil), unsafe.Slice(outBlob.Data, outBlob.Size)...)
	return base64.StdEncoding.EncodeToString(protected), nil
}

func (dpapiStore) Unprotect(ciphertext string) (string, error) {
	encoded, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("decode DPAPI token: %w", err)
	}
	if len(encoded) == 0 {
		return "", fmt.Errorf("decode DPAPI token: empty ciphertext")
	}

	inBlob := windows.DataBlob{Size: uint32(len(encoded)), Data: &encoded[0]}
	var outBlob windows.DataBlob
	if err := windows.CryptUnprotectData(&inBlob, nil, nil, 0, nil, 0, &outBlob); err != nil {
		return "", fmt.Errorf("DPAPI unprotect failed: %w", err)
	}
	defer freeDataBlob(&outBlob)

	return string(unsafe.Slice(outBlob.Data, outBlob.Size)), nil
}

func freeDataBlob(blob *windows.DataBlob) {
	if blob == nil || blob.Data == nil {
		return
	}
	_, _ = windows.LocalFree(windows.Handle(uintptr(unsafe.Pointer(blob.Data))))
	blob.Data = nil
	blob.Size = 0
}
