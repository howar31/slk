package commands

import (
	"fmt"
	"io"
	"strings"

	"golang.org/x/term"
)

// Test seams. Overridden in tests to simulate a terminal without a real TTY.
var (
	isTerminal = term.IsTerminal
	readSecret = term.ReadPassword
)

// promptLine writes prompt to w and reads one whitespace-trimmed line from r.
func promptLine(r io.Reader, w io.Writer, prompt string) (string, error) {
	fmt.Fprint(w, prompt)
	return readLine(r)
}

// readLine reads one whitespace-trimmed line from r, one byte at a time. Reading
// byte-by-byte (rather than via bufio) guarantees it never consumes past the
// newline, so a following term.ReadPassword on the same fd sees the bytes it
// should. Used for `--user -` and interactive prompts.
func readLine(r io.Reader) (string, error) {
	var b []byte
	var buf [1]byte
	for {
		n, err := r.Read(buf[:])
		if n > 0 {
			if buf[0] == '\n' {
				break
			}
			b = append(b, buf[0])
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return "", err
		}
	}
	return strings.TrimSpace(string(b)), nil
}

// promptSecret writes prompt to w and reads one line from fd with echo disabled.
// A newline is emitted to w afterwards because the terminal does not echo Enter.
func promptSecret(fd int, w io.Writer, prompt string) (string, error) {
	fmt.Fprint(w, prompt)
	b, err := readSecret(fd)
	fmt.Fprintln(w)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

// resolveLine resolves a non-secret field. Precedence: an explicit flag (changed)
// wins; otherwise prompt when interactive; otherwise empty.
func resolveLine(r io.Reader, w io.Writer, flagVal string, changed, interactive bool, prompt string) (string, error) {
	if changed {
		return flagVal, nil
	}
	if interactive {
		return promptLine(r, w, prompt)
	}
	return "", nil
}

// resolveSecret resolves a token field. Precedence: a flag value of "-" reads one
// line from stdin; any other explicit flag value wins; otherwise prompt (hidden)
// when interactive; otherwise empty. When required and interactive, an empty entry
// re-prompts.
func resolveSecret(r io.Reader, w io.Writer, fd int, flagVal string, changed, interactive, required bool, prompt string) (string, error) {
	if changed {
		if flagVal == "-" {
			return readLine(r)
		}
		return flagVal, nil
	}
	if !interactive {
		return "", nil
	}
	for {
		v, err := promptSecret(fd, w, prompt)
		if err != nil {
			return "", err
		}
		if v != "" || !required {
			return v, nil
		}
		fmt.Fprintln(w, "  a token is required")
	}
}
