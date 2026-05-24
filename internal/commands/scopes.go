package commands

import (
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// scopeUnion returns the sorted, de-duplicated set of scopes the command tree
// under root requires for identity ("user" or "bot"). It reads each command's
// "userScopes"/"botScopes" annotation (comma-separated; empty contributes
// nothing). This is the single source consumed by `auth login` (runtime) and
// the README manifest generator.
func scopeUnion(root *cobra.Command, identity string) []string {
	key := "userScopes"
	if identity == "bot" {
		key = "botScopes"
	}
	set := map[string]struct{}{}
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		for _, s := range strings.Split(c.Annotations[key], ",") {
			if s = strings.TrimSpace(s); s != "" {
				set[s] = struct{}{}
			}
		}
		for _, ch := range c.Commands() {
			walk(ch)
		}
	}
	walk(root)
	out := make([]string, 0, len(set))
	for s := range set {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}
