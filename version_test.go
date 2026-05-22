package slk

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestVersion_MatchesVersionFile(t *testing.T) {
	data, err := os.ReadFile("VERSION")
	if err != nil {
		t.Fatalf("read VERSION: %v", err)
	}
	want := strings.TrimSpace(string(data))
	if Version != want {
		t.Errorf("Version = %q, want %q", Version, want)
	}
}

func TestVersion_IsSemver(t *testing.T) {
	if !regexp.MustCompile(`^\d+\.\d+\.\d+`).MatchString(Version) {
		t.Errorf("Version %q is not semver", Version)
	}
}
