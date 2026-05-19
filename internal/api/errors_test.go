package api

import "testing"

func TestExitCodeFor(t *testing.T) {
	cases := map[string]int{
		"invalid_auth":      3,
		"token_expired":     3,
		"not_authed":        3,
		"account_inactive":  3,
		"channel_not_found": 4,
		"user_not_found":    4,
		"thread_not_found":  4,
		"message_not_found": 4,
		"ratelimited":       5,
		"something_else":    1,
	}
	for slackErr, want := range cases {
		if got := ExitCodeFor(slackErr); got != want {
			t.Errorf("ExitCodeFor(%q) = %d, want %d", slackErr, got, want)
		}
	}
}

func TestAPIError_Error(t *testing.T) {
	e := &APIError{SlackError: "channel_not_found", Method: "conversations.history"}
	if e.Error() != "conversations.history: channel_not_found" {
		t.Fatalf("unexpected message: %q", e.Error())
	}
	if e.ExitCode() != 4 {
		t.Fatalf("ExitCode = %d, want 4", e.ExitCode())
	}
}
