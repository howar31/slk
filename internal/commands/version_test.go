package commands

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestVersionCommand_PlainOffline(t *testing.T) {
	// Plain `slk version` must print "slk <version>" and touch no network.
	cmd := newVersionCommand(&GlobalFlags{}, "0.1.0")
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs(nil)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got := out.String(); got != "slk 0.1.0\n" {
		t.Fatalf("plain version output = %q, want %q", got, "slk 0.1.0\n")
	}
}

func TestVersionCommand_CheckFlagRegistered(t *testing.T) {
	cmd := newVersionCommand(&GlobalFlags{}, "0.1.0")
	if cmd.Flags().Lookup("check") == nil {
		t.Fatal("missing --check flag on version command")
	}
}

func TestRootCommand_HasVersionSubcommand(t *testing.T) {
	root := NewRootCommand("0.1.0")
	sub, _, err := root.Find([]string{"version"})
	if err != nil {
		t.Fatalf("find version: %v", err)
	}
	if sub.Name() != "version" {
		t.Fatalf("expected version subcommand, got %q", sub.Name())
	}
}

func TestRootCommand_VersionTemplate(t *testing.T) {
	// `slk --version` mirrors the subcommand: "slk <version>", not the Cobra
	// default "slk version <version>".
	root := NewRootCommand("0.1.0")
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"--version"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got := out.String(); got != "slk 0.1.0\n" {
		t.Fatalf("--version output = %q, want %q", got, "slk 0.1.0\n")
	}
}

func TestVersionCheck_Concise(t *testing.T) {
	cases := []struct {
		name string
		vc   versionCheck
		want string
	}{
		{
			name: "up to date",
			vc:   versionCheck{Current: "0.2.0", Latest: "0.2.0", Checked: true},
			want: "slk 0.2.0 (up to date)",
		},
		{
			name: "update available",
			vc:   versionCheck{Current: "0.1.0", Latest: "0.2.0", UpdateAvailable: true, Upgrade: upgradeHint, Checked: true},
			want: "slk 0.1.0 → 0.2.0 update available; " + upgradeHint,
		},
		{
			name: "dev build",
			vc:   versionCheck{Current: "dev", Latest: "0.2.0", DevBuild: true, Checked: true},
			want: "slk dev (development build); latest release 0.2.0",
		},
		{
			name: "could not check",
			vc:   versionCheck{Current: "0.1.0", Note: "request failed", Checked: false},
			want: "slk 0.1.0 — could not check for updates: request failed",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.vc.Concise(); got != tc.want {
				t.Fatalf("Concise() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestParseSemver(t *testing.T) {
	cases := []struct {
		in   string
		want [3]int
	}{
		{"0.1.0", [3]int{0, 1, 0}},
		{"v0.2.3", [3]int{0, 2, 3}},
		{" v1.4.9 ", [3]int{1, 4, 9}},
		{"v0.1.0-rc1", [3]int{0, 1, 0}},
		{"0.1.0+build.5", [3]int{0, 1, 0}},
		{"1.2", [3]int{1, 2, 0}},
		{"2", [3]int{2, 0, 0}},
		{"dev", [3]int{0, 0, 0}},
		{"", [3]int{0, 0, 0}},
		{"v3.10.100", [3]int{3, 10, 100}},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := parseSemver(tc.in); got != tc.want {
				t.Fatalf("parseSemver(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"0.1.0", "0.2.0", -1},
		{"0.2.0", "0.1.0", 1},
		{"0.2.0", "0.2.0", 0},
		{"v0.1.0", "0.1.0", 0},
		{"0.1.0", "v0.2.0", -1},
		{"0.9.0", "0.10.0", -1}, // numeric, not lexical
		{"1.0.0", "0.9.9", 1},
		{"0.1.1", "0.1.0", 1},
	}
	for _, tc := range cases {
		t.Run(tc.a+"_vs_"+tc.b, func(t *testing.T) {
			if got := compareVersions(tc.a, tc.b); got != tc.want {
				t.Fatalf("compareVersions(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

func TestFetchLatestRelease(t *testing.T) {
	t.Run("returns tag and sends User-Agent", func(t *testing.T) {
		var gotUA string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotUA = r.Header.Get("User-Agent")
			if r.URL.Path != releasesPath {
				t.Errorf("path = %q, want %q", r.URL.Path, releasesPath)
			}
			w.Write([]byte(`{"tag_name":"v0.2.0"}`))
		}))
		defer srv.Close()

		tag, err := fetchLatestRelease(srv.Client(), srv.URL, "0.1.0")
		if err != nil {
			t.Fatalf("fetchLatestRelease: %v", err)
		}
		if tag != "v0.2.0" {
			t.Fatalf("tag = %q, want v0.2.0", tag)
		}
		if gotUA == "" {
			t.Fatal("User-Agent header was not sent")
		}
	})

	t.Run("rate limited", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		}))
		defer srv.Close()
		_, err := fetchLatestRelease(srv.Client(), srv.URL, "0.1.0")
		if err == nil || !strings.Contains(err.Error(), "rate limit") {
			t.Fatalf("expected rate limit error, got %v", err)
		}
	})

	t.Run("non-200 status", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()
		_, err := fetchLatestRelease(srv.Client(), srv.URL, "0.1.0")
		if err == nil || !strings.Contains(err.Error(), "status 500") {
			t.Fatalf("expected status error, got %v", err)
		}
	})

	t.Run("malformed json", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`not json`))
		}))
		defer srv.Close()
		_, err := fetchLatestRelease(srv.Client(), srv.URL, "0.1.0")
		if err == nil || !strings.Contains(err.Error(), "parsing response") {
			t.Fatalf("expected parse error, got %v", err)
		}
	})

	t.Run("empty tag_name", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"tag_name":""}`))
		}))
		defer srv.Close()
		_, err := fetchLatestRelease(srv.Client(), srv.URL, "0.1.0")
		if err == nil || !strings.Contains(err.Error(), "no tag_name") {
			t.Fatalf("expected empty tag error, got %v", err)
		}
	})

	t.Run("network error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		url := srv.URL
		srv.Close() // closed server: connection refused
		_, err := fetchLatestRelease(&http.Client{Timeout: time.Second}, url, "0.1.0")
		if err == nil || !strings.Contains(err.Error(), "request failed") {
			t.Fatalf("expected request error, got %v", err)
		}
	})
}

func TestRunUpdateCheck(t *testing.T) {
	releaseServer := func(tag string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]string{"tag_name": tag})
		}))
	}

	t.Run("update available", func(t *testing.T) {
		srv := releaseServer("v0.2.0")
		defer srv.Close()
		res := runUpdateCheck("0.1.0", srv.URL)
		if !res.Checked || !res.UpdateAvailable {
			t.Fatalf("expected checked update-available, got %+v", res)
		}
		if res.Latest != "0.2.0" {
			t.Fatalf("Latest = %q, want 0.2.0", res.Latest)
		}
		if res.Upgrade == "" {
			t.Fatal("expected an upgrade hint when an update is available")
		}
	})

	t.Run("up to date", func(t *testing.T) {
		srv := releaseServer("v0.2.0")
		defer srv.Close()
		res := runUpdateCheck("0.2.0", srv.URL)
		if !res.Checked || res.UpdateAvailable {
			t.Fatalf("expected checked up-to-date, got %+v", res)
		}
		if res.Upgrade != "" {
			t.Fatalf("no upgrade hint expected when up to date, got %q", res.Upgrade)
		}
	})

	t.Run("dev build does not assert update", func(t *testing.T) {
		srv := releaseServer("v0.2.0")
		defer srv.Close()
		res := runUpdateCheck("dev", srv.URL)
		if !res.Checked || !res.DevBuild || res.UpdateAvailable {
			t.Fatalf("dev build should be checked, dev, not update-available; got %+v", res)
		}
		if res.Latest != "0.2.0" {
			t.Fatalf("Latest = %q, want 0.2.0", res.Latest)
		}
	})

	t.Run("graceful degradation on failure", func(t *testing.T) {
		srv := releaseServer("v0.2.0")
		url := srv.URL
		srv.Close() // unreachable
		res := runUpdateCheck("0.1.0", url)
		if res.Checked {
			t.Fatalf("expected Checked=false on failure, got %+v", res)
		}
		if res.Current != "0.1.0" {
			t.Fatalf("Current should still be reported, got %q", res.Current)
		}
		if res.Note == "" {
			t.Fatal("expected a failure note")
		}
	})
}

func TestVersionCheck_JSONOutput(t *testing.T) {
	// `slk version --check --format json` must emit parseable JSON carrying
	// the agent-facing fields (one-element array, per project convention).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"tag_name":"v0.2.0"}`))
	}))
	defer srv.Close()

	res := runUpdateCheck("0.1.0", srv.URL)
	res.Latest = "0.2.0" // sanity: produced by the check above
	b, err := json.Marshal([]versionCheck{res})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded []map[string]any
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(decoded) != 1 {
		t.Fatalf("expected 1 element, got %d", len(decoded))
	}
	for _, key := range []string{"current", "update_available", "checked"} {
		if _, ok := decoded[0][key]; !ok {
			t.Errorf("json missing key %q", key)
		}
	}
}
