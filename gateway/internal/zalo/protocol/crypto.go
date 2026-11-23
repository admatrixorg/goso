// Package protocol ports the Zalo personal chat wire format from GoClaw,
// which itself follows zcago (MIT). Zalo encrypts payloads with AES-CBC
// under a zero IV, and message bodies with a non standard 16 byte GCM nonce.
package protocol

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
)

var (
	ErrBlockSize  = errors.New("zalo personal: invalid block size")
	ErrPadding    = errors.New("zalo personal: invalid PKCS#7 padding")
	ErrCiphertext = errors.New("zalo personal: invalid ciphertext")
)

// DefaultUserAgent is the client GoClaw presents to Zalo web.
const DefaultUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36"

// DefaultAPIType and DefaultAPIVersion are the client identify Zalo expects.
const (
	DefaultAPIType    = 30
	DefaultAPIVersion = 671
)

// DefaultLanguage is the language code Zalo expects.
const DefaultLanguage = "vi"

// EncodeAESCBC encrypts data with AES-CBC under a zero IV, which is what the
// Zalo web endpoints expect. encHex picks hex instead of base64.
func EncodeAESCBC(key []byte, data string, encHex bool) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("new cipher: %w", err)
	}
	plain, err := pkcs7Pad([]byte(data), aes.BlockSize)
	if err != nil {
		return "", fmt.Errorf("pkcs7 pad: %w", err)
	}
	iv := make([]byte, aes.BlockSize)
	ct := make([]byte, len(plain))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(ct, plain)
	if encHex {
		return hex.EncodeToString(ct), nil
	}
	return base64.StdEncoding.EncodeToString(ct), nil
}

// DecodeAESCBC reverses EncodeAESCBC for base64 payloads.
func DecodeAESCBC(key []byte, data string) ([]byte, error) {
	ct, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return nil, fmt.Errorf("base64 decode: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("new cipher: %w", err)
	}
	if len(ct) == 0 || len(ct)%aes.BlockSize != 0 {
		return nil, ErrCiphertext
	}
	iv := make([]byte, aes.BlockSize)
	plain := make([]byte, len(ct))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(plain, ct)
	return pkcs7Unpad(plain, aes.BlockSize)
}

// DecodeAESGCM decodes a message body with Zalo's 16 byte nonce GCM.
func DecodeAESGCM(key, iv, aad, ct []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("new cipher: %w", err)
	}
	gcm, err := cipher.NewGCMWithNonceSize(block, 16)
	if err != nil {
		return nil, fmt.Errorf("new gcm: %w", err)
	}
	plain, err := gcm.Open(nil, iv, ct, aad)
	if err != nil {
		return nil, fmt.Errorf("gcm open: %w", err)
	}
	return plain, nil
}

// EncodeAESGCM produces a body with the same non standard nonce size.
func EncodeAESGCM(key, iv, aad, plain []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("new cipher: %w", err)
	}
	gcm, err := cipher.NewGCMWithNonceSize(block, 16)
	if err != nil {
		return nil, fmt.Errorf("new gcm: %w", err)
	}
	return gcm.Seal(nil, iv, plain, aad), nil
}

// DecodeSecretKey turns the base64 zpw_enk value into raw key bytes.
func DecodeSecretKey(secret string) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(secret)
	if err != nil {
		return nil, fmt.Errorf("secret key is not base64: %w", err)
	}
	if len(raw) == 0 {
		return nil, ErrCiphertext
	}
	return raw, nil
}

// ResolveKey picks the key length Zalo uses for a given secret.
func ResolveKey(raw []byte) ([]byte, bool) {
	switch len(raw) {
	case 16, 24, 32:
		return raw, true
	default:
		return nil, false
	}
}

func pkcs7Pad(data []byte, blockSize int) ([]byte, error) {
	if blockSize <= 0 {
		return nil, ErrBlockSize
	}
	padLen := blockSize - (len(data) % blockSize)
	return append(data, bytes.Repeat([]byte{byte(padLen)}, padLen)...), nil
}

func pkcs7Unpad(data []byte, blockSize int) ([]byte, error) {
	if blockSize <= 0 {
		return nil, ErrBlockSize
	}
	if len(data) == 0 || len(data)%blockSize != 0 {
		return nil, ErrPadding
	}
	padLen := int(data[len(data)-1])
	if padLen == 0 || padLen > blockSize || padLen > len(data) {
		return nil, ErrPadding
	}
	if !bytes.Equal(bytes.Repeat([]byte{byte(padLen)}, padLen), data[len(data)-padLen:]) {
		return nil, ErrPadding
	}
	return data[:len(data)-padLen], nil
}
