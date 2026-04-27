package apikey

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

const DefaultDevEncryptionKey = "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY"

var (
	ErrPlaintextUnavailable = errors.New("legacy key plaintext unavailable")
	ErrAPIKeyNotFound       = errors.New("api key not found")
)

type Cipher struct {
	aead cipher.AEAD
}

func NewCipher(raw string) (*Cipher, error) {
	decoded, err := decodeKey(raw)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(decoded)
	if err != nil {
		return nil, fmt.Errorf("build api key cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("build api key gcm: %w", err)
	}
	return &Cipher{aead: aead}, nil
}

func ValidateEncryptionKey(raw string) error {
	_, err := decodeKey(raw)
	return err
}

func (c *Cipher) Encrypt(plaintext string) (string, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}
	sealed := c.aead.Seal(nil, nonce, []byte(plaintext), nil)
	payload := append(nonce, sealed...)
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func (c *Cipher) Decrypt(encoded string) (string, error) {
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		return "", fmt.Errorf("decode api key ciphertext: %w", err)
	}
	nonceSize := c.aead.NonceSize()
	if len(payload) <= nonceSize {
		return "", fmt.Errorf("decode api key ciphertext: payload too short")
	}
	nonce := payload[:nonceSize]
	ciphertext := payload[nonceSize:]
	plaintext, err := c.aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt api key ciphertext: %w", err)
	}
	return string(plaintext), nil
}

func decodeKey(raw string) ([]byte, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, fmt.Errorf("API_KEY_ENCRYPTION_KEY must not be empty")
	}
	decoded, err := base64.RawURLEncoding.DecodeString(trimmed)
	if err != nil {
		return nil, fmt.Errorf("API_KEY_ENCRYPTION_KEY must be base64url encoded: %w", err)
	}
	if len(decoded) != 32 {
		return nil, fmt.Errorf("API_KEY_ENCRYPTION_KEY must decode to 32 bytes")
	}
	return decoded, nil
}
