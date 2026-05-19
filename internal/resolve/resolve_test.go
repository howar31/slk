package resolve

import (
	"path/filepath"
	"testing"
)

type fakeLookup struct{ calls int }

func (f *fakeLookup) Name(id string) (string, error) {
	f.calls++
	return "name-of-" + id, nil
}

func TestResolver_CachesLookups(t *testing.T) {
	fl := &fakeLookup{}
	r := New(filepath.Join(t.TempDir(), "cache.json"), fl)

	if got := r.Resolve("U123"); got != "name-of-U123" {
		t.Fatalf("first resolve = %q", got)
	}
	if got := r.Resolve("U123"); got != "name-of-U123" {
		t.Fatalf("second resolve = %q", got)
	}
	if fl.calls != 1 {
		t.Fatalf("expected 1 lookup (cached), got %d", fl.calls)
	}
}

func TestResolver_DegradesOnError(t *testing.T) {
	r := New(filepath.Join(t.TempDir(), "cache.json"), failLookup{})
	if got := r.Resolve("U999"); got != "U999" {
		t.Fatalf("expected raw ID fallback, got %q", got)
	}
}

type failLookup struct{}

func (failLookup) Name(string) (string, error) { return "", errFail }

var errFail = errTest("lookup failed")

type errTest string

func (e errTest) Error() string { return string(e) }

func TestResolver_PersistsCacheAcrossInstances(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.json")
	fl := &fakeLookup{}
	New(path, fl).Resolve("U123") // warm in-memory cache and write to disk
	r2 := New(path, fl)           // cold start, should load cache from disk
	r2.Resolve("U123")            // should hit the disk-loaded cache, not a live lookup
	if fl.calls != 1 {
		t.Fatalf("expected 1 live lookup across two Resolver instances, got %d", fl.calls)
	}
}
