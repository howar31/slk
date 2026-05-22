package auth

import (
	"os"
	"testing"
)

// TestMain guarantees no auth-package test ever touches the real OS keyring:
// the default keyring is replaced with an in-memory fake. Individual tests may
// still override activeKeyring via withFakeKeyring.
func TestMain(m *testing.M) {
	activeKeyring = newFakeKeyring()
	os.Exit(m.Run())
}
