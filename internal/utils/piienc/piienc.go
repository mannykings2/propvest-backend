// Package piienc encrypts PII (BVN/NIN) at rest with AES-256-GCM.
//
// Why at rest, now (Phase 6.1): kyc_submissions.bvn/nin were plaintext
// columns. Anyone with a DB dump, a leaked read-replica credential or a
// over-broad support query held a customer's BVN in the clear. The database
// no longer stores anything recognisable: values are written as
//
//	v1:<base64url(nonce || ciphertext)>
//
// The nonce is random per encryption, so encrypting the same BVN twice
// produces different ciphertext (no equality leaks across rows), and GCM
// authenticates the value: Decrypt fails loudly on tampering or on a key
// rotation that no longer matches, rather than returning garbage.
//
// Key: KYC_ENCRYPTION_KEY (any non-empty string; SHA-256'd to a 32-byte
// key, so operators pick a passphrase, not raw key bytes). Production boot
// refuses to run without one (config.Validate); development falls back to a
// fixed key with a warning — never ship that fallback.
//
// Legacy plaintext: rows written before this migration are 11 raw digits.
// Decrypt passes values without the "v1:" prefix through unchanged, and the
// KYC repository re-encrypts them on next read (lazy backfill), so no
// one-off data migration is needed.
package piienc

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
)

// prefix marks an encrypted value. Anything without it is legacy plaintext.
const prefix = "v1:"

// ErrEmptyKey is returned when the configured key is empty.
var ErrEmptyKey = errors.New("piienc: key must not be empty")

// ErrDecryptFailed covers wrong key, corrupted or truncated values.
var ErrDecryptFailed = errors.New("piienc: decrypt failed")

// Cipher encrypts/decrypts PII fields with AES-256-GCM.
type Cipher struct {
	aead cipher.AEAD
}

// New builds a Cipher from a passphrase. The passphrase is SHA-256-hashed to
// fill the 32-byte AES key, so any operator-chosen secret length works.
func New(key string) (*Cipher, error) {
	if strings.TrimSpace(key) == "" {
		return nil, ErrEmptyKey
	}
	sum := sha256.Sum256([]byte(key))
	block, err := aes.NewCipher(sum[:])
	if err != nil {
		return nil, fmt.Errorf("piienc: new cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("piienc: new aead: %w", err)
	}
	return &Cipher{aead: aead}, nil
}

// Encrypt returns prefix + base64url(nonce || ciphertext) for the plaintext.
// The empty string encrypts to a valid token (a missing value stays
// undistinguishable from any other encrypted value in the DB).
func (c *Cipher) Encrypt(plaintext string) (string, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("piienc: nonce: %w", err)
	}
	sealed := c.aead.Seal(nonce, nonce, []byte(plaintext), nil)
	return prefix + base64.RawURLEncoding.EncodeToString(sealed), nil
}

// Decrypt reverses Encrypt. A value without the prefix is returned unchanged
// (legacy plaintext row — the caller's lazy backfill re-encrypts it).
func (c *Cipher) Decrypt(stored string) (string, error) {
	if !IsEncrypted(stored) {
		return stored, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(stored, prefix))
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrDecryptFailed, err)
	}
	ns := c.aead.NonceSize()
	if len(raw) < ns {
		return "", ErrDecryptFailed
	}
	plain, err := c.aead.Open(nil, raw[:ns], raw[ns:], nil)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrDecryptFailed, err)
	}
	return string(plain), nil
}

// IsEncrypted reports whether a stored value carries the v1: token prefix.
func IsEncrypted(stored string) bool {
	return strings.HasPrefix(stored, prefix)
}
