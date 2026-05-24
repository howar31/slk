package auth

import "fmt"

// EncryptionStatus returns a single non-secret line describing at-rest
// encryption for display by `slk auth status`. It never includes any token,
// secret, or key material.
func EncryptionStatus(cfg *Config) string {
	if cfg.KeyBackend == "" {
		return "encryption: none (legacy plaintext)"
	}
	for _, p := range cfg.Profiles {
		if isEncrypted(p.Token) {
			return fmt.Sprintf("encryption: %s (error: key unavailable)", cfg.KeyBackend)
		}
	}
	return fmt.Sprintf("encryption: %s (ok)", cfg.KeyBackend)
}
