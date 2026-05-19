package api

import "fmt"

// APIError is a structured failure from a Slack Web API call.
type APIError struct {
	Method     string // Slack method, e.g. "conversations.history"
	SlackError string // Slack "error" field, e.g. "channel_not_found"
}

func (e *APIError) Error() string {
	return fmt.Sprintf("%s: %s", e.Method, e.SlackError)
}

// ExitCode returns the process exit code for this error.
func (e *APIError) ExitCode() int { return ExitCodeFor(e.SlackError) }

// ExitCodeFor maps a Slack error string to a slk exit code.
func ExitCodeFor(slackErr string) int {
	switch slackErr {
	case "invalid_auth", "token_expired", "not_authed", "account_inactive":
		return 3
	case "channel_not_found", "user_not_found", "thread_not_found", "message_not_found":
		return 4
	case "ratelimited", "rate_limited":
		return 5
	default:
		return 1
	}
}
