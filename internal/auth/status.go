package auth

import "fmt"

// EncryptionInfo returns the at-rest encryption backend and status for
// `slk auth status --format json`. backend is the key backend ("file"/"keyring")
// or "none" for legacy plaintext; status is "ok", "key_unavailable", or "none".
// It never includes any token, secret, or key material.
func EncryptionInfo(cfg *Config) (backend, status string) {
	if cfg.KeyBackend == "" {
		return "none", "none"
	}
	for _, p := range cfg.Profiles {
		if isEncrypted(p.Token) {
			return cfg.KeyBackend, "key_unavailable"
		}
	}
	return cfg.KeyBackend, "ok"
}

// EncryptionStatus returns a single non-secret line describing at-rest
// encryption for display by `slk auth status`. It never includes any token,
// secret, or key material.
func EncryptionStatus(cfg *Config) string {
	backend, status := EncryptionInfo(cfg)
	switch status {
	case "none":
		return "encryption: none (legacy plaintext)"
	case "key_unavailable":
		return fmt.Sprintf("encryption: %s (error: key unavailable)", backend)
	default:
		return fmt.Sprintf("encryption: %s (ok)", backend)
	}
}
