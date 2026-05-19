package auth

// AuthError is a credential-resolution failure. It carries exit code 3.
type AuthError struct {
	Reason string
}

func (e *AuthError) Error() string { return e.Reason }

// ExitCode returns the process exit code for an auth error.
func (e *AuthError) ExitCode() int { return 3 }
