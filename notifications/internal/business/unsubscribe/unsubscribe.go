// Package unsubscribe seals an address into an opaque link token and opens
// it again, so the link identifies the address without exposing it.
package unsubscribe

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
)

// ErrInvalid means the token was not sealed with this key.
var ErrInvalid = errors.New("invalid unsubscribe token")

// Sealer seals and opens tokens with AES-GCM.
type Sealer struct {
	aead cipher.AEAD
}

// New constructs a Sealer from a 32-byte key.
func New(key []byte) (*Sealer, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("unsubscribe key must be 32 bytes, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Sealer{aead: aead}, nil
}

// Seal returns the token of email.
func (s *Sealer) Seal(email string) (string, error) {
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(s.aead.Seal(nonce, nonce, []byte(email), nil)), nil
}

// Open returns the address sealed in token.
func (s *Sealer) Open(token string) (string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(raw) < s.aead.NonceSize() {
		return "", ErrInvalid
	}
	nonce, sealed := raw[:s.aead.NonceSize()], raw[s.aead.NonceSize():]
	plain, err := s.aead.Open(nil, nonce, sealed, nil)
	if err != nil {
		return "", ErrInvalid
	}
	return string(plain), nil
}
