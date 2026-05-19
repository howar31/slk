// Command slk is an agent-facing Slack CLI.
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/howar31/slk/internal/api"
	"github.com/howar31/slk/internal/auth"
	"github.com/howar31/slk/internal/commands"
)

// version is overridden at build time via -ldflags.
var version = "dev"

func main() {
	root := commands.NewRootCommand(version)
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "slk:", err)
		var apiErr *api.APIError
		if errors.As(err, &apiErr) {
			os.Exit(apiErr.ExitCode())
		}
		var authErr *auth.AuthError
		if errors.As(err, &authErr) {
			os.Exit(authErr.ExitCode())
		}
		os.Exit(1)
	}
}
