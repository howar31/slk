package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"strings"
)

// encPrefix marks an encrypted field value and versions the scheme.
const encPrefix = "enc:v1:"

// isEncrypted reports whether v is an encrypted field value.
func isEncrypted(v string) bool { return strings.HasPrefix(v, encPrefix) }

// IsEncrypted reports whether v is still an slk-encrypted (ciphertext) value.
// Exposed for callers that must skip an unusable token Load could not decrypt
// (key unavailable), e.g. `auth status`.
func IsEncrypted(v string) bool { return isEncrypted(v) }

// encryptValue seals plaintext with AES-256-GCM and returns
// enc:v1:<base64(nonce||ciphertext||tag)>. key must be 32 bytes.
func encryptValue(key []byte, plaintext string) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return encPrefix + base64.StdEncoding.EncodeToString(sealed), nil
}

// decryptValue reverses encryptValue. It returns an error if the value is not
// valid ciphertext or fails GCM authentication (wrong key / tampering).
func decryptValue(key []byte, value string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, encPrefix))
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", errors.New("ciphertext too short")
	}
	nonce, ct := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	pt, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", err
	}
	return string(pt), nil
}
