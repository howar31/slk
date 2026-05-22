package auth

import (
	"crypto/rand"
	"strings"
	"testing"
)

func testKey(t *testing.T) []byte {
	t.Helper()
	k := make([]byte, 32)
	if _, err := rand.Read(k); err != nil {
		t.Fatalf("rand: %v", err)
	}
	return k
}

func TestCrypto_EncryptDecryptRoundTrip(t *testing.T) {
	key := testKey(t)
	const plain = "xoxp-secret-value"
	enc, err := encryptValue(key, plain)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if !strings.HasPrefix(enc, encPrefix) {
		t.Fatalf("ciphertext missing %q prefix: %q", encPrefix, enc)
	}
	if strings.Contains(enc, plain) {
		t.Fatalf("ciphertext leaked plaintext: %q", enc)
	}
	got, err := decryptValue(key, enc)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if got != plain {
		t.Fatalf("round trip = %q, want %q", got, plain)
	}
}

func TestCrypto_DecryptWithWrongKeyFails(t *testing.T) {
	enc, err := encryptValue(testKey(t), "xoxp-secret")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if _, err := decryptValue(testKey(t), enc); err == nil {
		t.Fatal("decrypt with a different key should fail (GCM auth)")
	}
}

func TestCrypto_NonceIsUniquePerEncryption(t *testing.T) {
	key := testKey(t)
	a, _ := encryptValue(key, "same")
	b, _ := encryptValue(key, "same")
	if a == b {
		t.Fatal("two encryptions of the same plaintext must differ (random nonce)")
	}
}

func TestCrypto_IsEncrypted(t *testing.T) {
	if !isEncrypted(encPrefix + "abc") {
		t.Fatal("prefixed value should be detected as encrypted")
	}
	if isEncrypted("xoxp-plain") {
		t.Fatal("plaintext token must not be detected as encrypted")
	}
	if isEncrypted("") {
		t.Fatal("empty string is not encrypted")
	}
}
