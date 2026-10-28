package piienc

import (
	"strings"
	"testing"
)

func newTestCipher(t *testing.T) *Cipher {
	t.Helper()
	c, err := New("test-secret")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	c := newTestCipher(t)

	for _, plain := range []string{"12345678901", "98765432109", ""} {
		enc, err := c.Encrypt(plain)
		if err != nil {
			t.Fatalf("Encrypt(%q): %v", plain, err)
		}
		if !IsEncrypted(enc) {
			t.Fatalf("Encrypt(%q) = %q, missing %q prefix", plain, enc, prefix)
		}
		if strings.Contains(enc, plain) && plain != "" {
			t.Fatalf("ciphertext %q leaks plaintext %q", enc, plain)
		}
		got, err := c.Decrypt(enc)
		if err != nil {
			t.Fatalf("Decrypt(%q): %v", enc, err)
		}
		if got != plain {
			t.Fatalf("round trip: got %q, want %q", got, plain)
		}
	}
}

// The same BVN stored for two users must not produce identical ciphertext —
// a unique nonce per encryption guarantees that.
func TestEncryptRandomised(t *testing.T) {
	c := newTestCipher(t)

	a, err := c.Encrypt("12345678901")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	b, err := c.Encrypt("12345678901")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if a == b {
		t.Fatal("two encryptions of the same value are identical — nonce is not random")
	}
}

// Pre-migration rows hold 11 raw digits; Decrypt must pass them through so
// the repository's lazy backfill can re-encrypt on read.
func TestDecryptLegacyPlaintextPassthrough(t *testing.T) {
	c := newTestCipher(t)

	got, err := c.Decrypt("12345678901")
	if err != nil {
		t.Fatalf("Decrypt(legacy): %v", err)
	}
	if got != "12345678901" {
		t.Fatalf("legacy passthrough: got %q", got)
	}
}

// A different key must fail loudly, not return garbage — GCM authentication
// is what makes a key rotation detectable instead of silent corruption.
func TestDecryptWrongKeyFails(t *testing.T) {
	a := newTestCipher(t)
	b, err := New("other-secret")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	enc, err := a.Encrypt("12345678901")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if _, err := b.Decrypt(enc); err == nil {
		t.Fatal("Decrypt with wrong key succeeded — ciphertext is not authenticated")
	}
}

// Tampering (a flipped byte in the base64 payload) must fail authentication.
func TestDecryptTamperedFails(t *testing.T) {
	c := newTestCipher(t)

	enc, err := c.Encrypt("12345678901")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	tampered := prefix + "A" + enc[len(prefix)+1:]
	if _, err := c.Decrypt(tampered); err == nil {
		t.Fatal("Decrypt of a tampered value succeeded")
	}
}

func TestNewRejectsEmptyKey(t *testing.T) {
	for _, key := range []string{"", "   "} {
		if _, err := New(key); err == nil {
			t.Fatalf("New(%q) succeeded, want ErrEmptyKey", key)
		}
	}
}
