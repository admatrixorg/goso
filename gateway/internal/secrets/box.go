// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/store"
)

var (
	// ErrNoMasterKey is returned when GOSO_MASTER_KEY is empty or not 32-byte hex.
	ErrNoMasterKey = errors.New("master key required")
	// ErrName is returned for an empty secret name.
	ErrName = errors.New("secret name is required")
)

func masterKey() ([]byte, error) {
	raw := strings.TrimSpace(os.Getenv("GOSO_MASTER_KEY"))
	if raw == "" {
		return nil, ErrNoMasterKey
	}
	key, err := hex.DecodeString(raw)
	if err != nil || len(key) != 32 {
		return nil, ErrNoMasterKey
	}
	return key, nil
}

func gcmFor(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// Encrypt encrypts plaintext with AES-256-GCM. Nonce is random.
func Encrypt(plaintext []byte) (nonce, ct []byte, err error) {
	key, err := masterKey()
	if err != nil {
		return nil, nil, err
	}
	aead, err := gcmFor(key)
	if err != nil {
		return nil, nil, err
	}
	nonce = make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, err
	}
	ct = aead.Seal(nil, nonce, plaintext, nil)
	return nonce, ct, nil
}

// Decrypt recovers plaintext from nonce+ct.
func Decrypt(nonce, ct []byte) ([]byte, error) {
	key, err := masterKey()
	if err != nil {
		return nil, err
	}
	aead, err := gcmFor(key)
	if err != nil {
		return nil, err
	}
	if len(nonce) != aead.NonceSize() {
		return nil, fmt.Errorf("bad nonce")
	}
	return aead.Open(nil, nonce, ct, nil)
}

// Put encrypts a provider key blob and persists it. Empty master key refuses store.
func Put(st store.StoreIface, name string, plaintext []byte) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return ErrName
	}
	if st == nil {
		return errors.New("store required")
	}
	nonce, ct, err := Encrypt(plaintext)
	if err != nil {
		return err
	}
	return st.PutSecret(store.SecretRow{Name: name, Nonce: nonce, CT: ct})
}

// Get decrypts a stored provider key blob.
func Get(st store.StoreIface, name string) ([]byte, error) {
	if st == nil {
		return nil, errors.New("store required")
	}
	row, err := st.GetSecret(name)
	if err != nil {
		return nil, err
	}
	return Decrypt(row.Nonce, row.CT)
}

// RandomKeyHex returns a 32-byte key as hex (tests).
func RandomKeyHex() (string, error) {
	var b [32]byte
	if _, err := io.ReadFull(rand.Reader, b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
