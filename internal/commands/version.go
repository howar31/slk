package commands

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/howar31/slk/internal/output"
	"github.com/spf13/cobra"
)

const (
	// githubAPIBase is the GitHub REST API root used by the update check.
	// Overridable in tests via fetchLatestRelease's baseURL argument.
	githubAPIBase = "https://api.github.com"
	// releasesPath is the latest-release endpoint for this repository.
	releasesPath = "/repos/howar31/slk/releases/latest"
	// upgradeHint tells the user how to obtain a newer build. slk never
	// updates itself; upgrades stay the package manager's responsibility.
	upgradeHint = "brew upgrade slk  (or: npm i -g @howar31/slk@latest)"
	// checkTimeout bounds the update check so an unreachable network does
	// not stall an agent waiting on the result.
	checkTimeout = 5 * time.Second
)

// versionCheck is the result of `slk version --check`.
type versionCheck struct {
	Current         string `json:"current"`
	Latest          string `json:"latest,omitempty"`
	UpdateAvailable bool   `json:"update_available"`
	DevBuild        bool   `json:"dev_build,omitempty"`
	Upgrade         string `json:"upgrade,omitempty"`
	Checked         bool   `json:"checked"`
	Note            string `json:"note,omitempty"`
}

func (v versionCheck) Concise() string {
	switch {
	case !v.Checked:
		return fmt.Sprintf("slk %s — could not check for updates: %s", v.Current, v.Note)
	case v.DevBuild:
		return fmt.Sprintf("slk %s (development build); latest release %s", v.Current, v.Latest)
	case v.UpdateAvailable:
		return fmt.Sprintf("slk %s → %s update available; %s", v.Current, v.Latest, v.Upgrade)
	default:
		return fmt.Sprintf("slk %s (up to date)", v.Current)
	}
}

// newVersionCommand builds the `version` subcommand. Plain `slk version`
// prints the build version offline; `slk version --check` performs a single
// read-only request to GitHub Releases. version is the build-time version
// string threaded down from the root command.
func newVersionCommand(g *GlobalFlags, version string) *cobra.Command {
	var check bool
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print the slk version, or check for updates with --check",
		Long: `Print the running slk version.

With --check, slk performs a single read-only HTTP request to the GitHub
Releases API and reports whether a newer version is available, plus the
command to upgrade. The check never downloads or replaces the binary —
upgrades remain your package manager's job. Plain "slk version" stays
fully offline.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			if !check {
				fmt.Fprintf(out, "slk %s\n", version)
				return nil
			}
			result := runUpdateCheck(version, githubAPIBase)
			return output.Emit(out, g.Format, []versionCheck{result})
		},
	}
	cmd.Flags().BoolVar(&check, "check", false, "check GitHub Releases for a newer version (read-only; never self-updates)")
	return cmd
}

// runUpdateCheck performs the read-only update check and always returns a
// populated versionCheck. Failures are captured in the struct (Checked=false,
// Note set) rather than returned, so the command degrades gracefully instead
// of aborting — the caller's current version is reported either way.
func runUpdateCheck(current, baseURL string) versionCheck {
	res := versionCheck{Current: current, DevBuild: isDevBuild(current)}

	client := &http.Client{Timeout: checkTimeout}
	latest, err := fetchLatestRelease(client, baseURL, current)
	if err != nil {
		res.Note = err.Error()
		return res
	}
	res.Checked = true
	res.Latest = strings.TrimPrefix(latest, "v")

	// A development build has no released version to compare against: report
	// the latest release but never assert that an update is available.
	if res.DevBuild {
		return res
	}
	if compareVersions(current, res.Latest) < 0 {
		res.UpdateAvailable = true
		res.Upgrade = upgradeHint
	}
	return res
}

// isDevBuild reports whether current is a locally built binary rather than a
// released version. The build default is "dev" (see cmd/slk/main.go).
func isDevBuild(current string) bool {
	return current == "dev" || current == ""
}

// fetchLatestRelease returns the latest release tag (e.g. "v0.2.0") for the
// slk repository. baseURL is injectable so tests can point at a local server.
func fetchLatestRelease(client *http.Client, baseURL, current string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, baseURL+releasesPath, nil)
	if err != nil {
		return "", err
	}
	// GitHub rejects API requests that omit a User-Agent header.
	req.Header.Set("User-Agent", "slk/"+current)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}
	switch {
	case resp.StatusCode == http.StatusForbidden, resp.StatusCode == http.StatusTooManyRequests:
		return "", fmt.Errorf("GitHub API rate limit reached")
	case resp.StatusCode != http.StatusOK:
		return "", fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}
	var rel struct {
		TagName string `json:"tag_name"`
	}
	if err := json.Unmarshal(body, &rel); err != nil {
		return "", fmt.Errorf("parsing response: %w", err)
	}
	if rel.TagName == "" {
		return "", fmt.Errorf("no tag_name in GitHub response")
	}
	return rel.TagName, nil
}

// compareVersions returns -1 if a < b, 0 if equal, 1 if a > b. Inputs may
// carry a leading "v"; pre-release / build suffixes are ignored. Unparseable
// numeric components compare as zero.
func compareVersions(a, b string) int {
	pa, pb := parseSemver(a), parseSemver(b)
	for i := 0; i < 3; i++ {
		switch {
		case pa[i] < pb[i]:
			return -1
		case pa[i] > pb[i]:
			return 1
		}
	}
	return 0
}

// parseSemver extracts major.minor.patch from s, tolerating a leading "v" and
// a trailing pre-release / build suffix. Missing or non-numeric parts are 0.
func parseSemver(s string) [3]int {
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		s = s[:i]
	}
	var out [3]int
	for i, part := range strings.Split(s, ".") {
		if i >= 3 {
			break
		}
		out[i], _ = strconv.Atoi(part)
	}
	return out
}
