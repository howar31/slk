package auth

import "strings"

// TokenScope derives the Slack identity scope from a token's prefix: "user" for
// xoxp-, "bot" for xoxb-, "" for any other prefix (slk supports only these two).
// The token must be decrypted plaintext; a still-encrypted value (IsEncrypted)
// has no readable prefix and yields "".
func TokenScope(token string) string {
	switch {
	case strings.HasPrefix(token, "xoxp-"):
		return "user"
	case strings.HasPrefix(token, "xoxb-"):
		return "bot"
	default:
		return ""
	}
}
