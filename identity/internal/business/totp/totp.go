// Package totp holds the second factor's rules: time-based one-time codes
// (RFC 6238, SHA-1, 6 digits, 30 s steps), sealing their secrets at rest,
// the lockout after wrong codes, and the step-up proofs a valid code buys.
package totp

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"time"
)

const (
	// Step is how long one code is current.
	Step = 30 * time.Second
	// Digits is how many digits a code has.
	Digits = 6
	// Skew is how many steps before or after the current one are accepted,
	// for clocks that drift.
	Skew = 1
	// SecretBytes is the secret's length.
	SecretBytes = 20
	// Issuer is how authenticator apps label the credential.
	Issuer = "Playhoot"

	// MaxFailures wrong codes within FailureWindow lock the account for
	// LockDuration after the last of them.
	MaxFailures   = 5
	FailureWindow = 15 * time.Minute
	LockDuration  = 15 * time.Minute

	// PendingLifetime is how long a started enrollment waits for its first
	// code.
	PendingLifetime = 10 * time.Minute
	// ProofLifetime is how long a step-up proof stays spendable.
	ProofLifetime = 10 * time.Minute
	// MaxActionLength bounds the name of what a proof is spent on.
	MaxActionLength = 64
)

// KeyBytes is the length of the key secrets are sealed with (AES-256).
const KeyBytes = 32

var base32NoPad = base32.StdEncoding.WithPadding(base32.NoPadding)

// NewSecret returns a random secret, base32-encoded as authenticator apps
// expect it.
func NewSecret() (string, error) {
	raw := make([]byte, SecretBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generating totp secret: %s", err)
	}
	return base32NoPad.EncodeToString(raw), nil
}

// URI is the otpauth address an authenticator app enrolls from.
func URI(secret string, account string) string {
	label := url.PathEscape(Issuer + ":" + account)
	query := url.Values{"secret": {secret}, "issuer": {Issuer}, "algorithm": {"SHA1"}, "digits": {"6"}, "period": {"30"}}
	return "otpauth://totp/" + label + "?" + query.Encode()
}

// StepAt is the step number current at t.
func StepAt(t time.Time) int64 {
	return t.Unix() / int64(Step/time.Second)
}

// CodeAt returns the code of secret for step.
func CodeAt(secret string, step int64) (string, error) {
	key, err := base32NoPad.DecodeString(secret)
	if err != nil {
		return "", fmt.Errorf("decoding totp secret: %s", err)
	}
	var counter [8]byte
	binary.BigEndian.PutUint64(counter[:], uint64(step))
	mac := hmac.New(sha1.New, key)
	mac.Write(counter[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	value := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
	return fmt.Sprintf("%06d", value%1000000), nil
}

// Match returns the step within Skew of now whose code is code, or false.
// A malformed code never matches.
func Match(secret string, code string, now time.Time) (int64, bool, error) {
	if !wellFormedCode(code) {
		return 0, false, nil
	}
	current := StepAt(now)
	for step := current - Skew; step <= current+Skew; step++ {
		expected, err := CodeAt(secret, step)
		if err != nil {
			return 0, false, err
		}
		if hmac.Equal([]byte(expected), []byte(code)) {
			return step, true, nil
		}
	}
	return 0, false, nil
}

func wellFormedCode(code string) bool {
	if len(code) != Digits {
		return false
	}
	for _, c := range code {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// LockedUntil returns when the lock ends, given the newest wrong codes first
// (at most MaxFailures of them), or false when there is no lock at now.
func LockedUntil(failures []time.Time, now time.Time) (time.Time, bool) {
	if len(failures) < MaxFailures {
		return time.Time{}, false
	}
	newest, oldest := failures[0], failures[MaxFailures-1]
	if newest.Sub(oldest) > FailureWindow {
		return time.Time{}, false
	}
	until := newest.Add(LockDuration)
	return until, now.Before(until)
}

// Sealer seals secrets at rest with one key.
type Sealer struct {
	aead cipher.AEAD
}

// NewSealer returns a Sealer for key, which must have KeyBytes.
func NewSealer(key []byte) (*Sealer, error) {
	if len(key) != KeyBytes {
		return nil, fmt.Errorf("totp key must have %d bytes", KeyBytes)
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

// Seal encrypts secret.
func (s *Sealer) Seal(secret string) (string, error) {
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("generating nonce: %s", err)
	}
	return base64.StdEncoding.EncodeToString(s.aead.Seal(nonce, nonce, []byte(secret), nil)), nil
}

// ErrUnsealable means a sealed secret is corrupt or sealed with another key.
var ErrUnsealable = errors.New("totp secret cannot be unsealed")

// Open decrypts a sealed secret.
func (s *Sealer) Open(sealed string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(sealed)
	if err != nil || len(raw) < s.aead.NonceSize() {
		return "", ErrUnsealable
	}
	plain, err := s.aead.Open(nil, raw[:s.aead.NonceSize()], raw[s.aead.NonceSize():], nil)
	if err != nil {
		return "", ErrUnsealable
	}
	return string(plain), nil
}

// proofBytes is a step-up proof's entropy.
const proofBytes = 32

// NewProof returns a random step-up proof.
func NewProof() (string, error) {
	raw := make([]byte, proofBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generating step-up proof: %s", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// WellFormedProof reports whether proof could be one NewProof returned.
func WellFormedProof(proof string) bool {
	raw, err := base64.RawURLEncoding.DecodeString(proof)
	return err == nil && len(raw) == proofBytes
}

// HashProof is what is stored of a proof.
func HashProof(proof string) string {
	sum := sha256.Sum256([]byte(proof))
	return hex.EncodeToString(sum[:])
}

// ValidAction reports whether action may name what a proof is spent on.
func ValidAction(action string) bool {
	return action != "" && len(action) <= MaxActionLength
}
