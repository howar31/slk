package api

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

const testToken = "xoxp-test-token"

// newFileServerClient returns a client whose file allowlist holds only the
// TLS test server's own host (127.0.0.1). Every connection dials that server
// whatever the URL says, and certificates are not verified, so a redirect to
// any host lands back on handler with its Host header intact.
func newFileServerClient(t *testing.T, handler http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewTLSServer(handler)
	t.Cleanup(srv.Close)
	transport := srv.Client().Transport.(*http.Transport).Clone()
	transport.TLSClientConfig.InsecureSkipVerify = true
	transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, srv.Listener.Addr().String())
	}
	c := New(testToken)
	c.HTTP = &http.Client{Transport: transport}
	c.FileHosts = []string{"127.0.0.1"}
	return c, srv
}

func TestNew_AllowsFilesSlackCom(t *testing.T) {
	c := New(testToken)
	if err := c.checkFileURL("https://files.slack.com/files-pri/T0-F0/image.png"); err != nil {
		t.Fatalf("files.slack.com rejected: %v", err)
	}
}

func TestCheckFileURL_RejectsUntrustedURLs(t *testing.T) {
	cases := []struct{ name, url string }{
		{"external host", "https://cdn.example.com/image.png"},
		{"allowed name as subdomain prefix", "https://files.slack.com.example.com/image.png"},
		{"allowed name in path", "https://example.com/files.slack.com/image.png"},
		{"parent domain", "https://slack.com/image.png"},
		{"sibling subdomain", "https://evil.files.slack.com/image.png"},
		{"plain http", "http://files.slack.com/image.png"},
		{"userinfo disguise", "https://files.slack.com@example.com/image.png"},
		{"empty", ""},
	}
	c := New(testToken)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := c.checkFileURL(tc.url); err == nil {
				t.Fatalf("checkFileURL(%q) accepted an untrusted URL", tc.url)
			}
		})
	}
}

func TestOpenPrivateFile_SendsBearerToAllowedHost(t *testing.T) {
	var gotAuth atomic.Value
	c, srv := newFileServerClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth.Store(r.Header.Get("Authorization"))
		io.WriteString(w, "file-bytes")
	})

	resp, err := c.OpenPrivateFile(srv.URL + "/files-pri/image.png")
	if err != nil {
		t.Fatalf("OpenPrivateFile: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "file-bytes" {
		t.Errorf("body = %q, want file-bytes", body)
	}
	if gotAuth.Load() != "Bearer "+testToken {
		t.Errorf("Authorization = %q", gotAuth.Load())
	}
}

func TestOpenPrivateFile_DisallowedHostMakesNoRequest(t *testing.T) {
	var hits atomic.Int32
	c, srv := newFileServerClient(t, func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
	})
	c.FileHosts = []string{"files.slack.com"}

	_, err := c.OpenPrivateFile(srv.URL + "/image.png")
	if err == nil {
		t.Fatal("expected an error for a host outside the allowlist")
	}
	if hits.Load() != 0 {
		t.Errorf("server received %d requests, want 0", hits.Load())
	}
	if strings.Contains(err.Error(), testToken) {
		t.Errorf("error leaks the token: %v", err)
	}
}

// A subdomain is the case that needs slk's own check: net/http keeps the
// Authorization header on redirects to a subdomain of the original host.
func TestOpenPrivateFile_RedirectToSubdomainDropsToken(t *testing.T) {
	var redirectedAuth, redirectedHost atomic.Value
	c, _ := newFileServerClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/start" {
			http.Redirect(w, r, "https://cdn.example.com/landing", http.StatusFound)
			return
		}
		redirectedHost.Store(r.Host)
		redirectedAuth.Store(r.Header.Get("Authorization"))
	})
	c.FileHosts = []string{"example.com"}

	resp, err := c.OpenPrivateFile("https://example.com/start")
	if err != nil {
		t.Fatalf("OpenPrivateFile: %v", err)
	}
	resp.Body.Close()

	if redirectedHost.Load() != "cdn.example.com" {
		t.Fatalf("redirect did not reach cdn.example.com, host = %v", redirectedHost.Load())
	}
	if redirectedAuth.Load() != "" {
		t.Errorf("token forwarded to a disallowed host: %q", redirectedAuth.Load())
	}
}

func TestOpenPrivateFile_RedirectWithinAllowedHostKeepsToken(t *testing.T) {
	var finalAuth atomic.Value
	c, srv := newFileServerClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/start" {
			http.Redirect(w, r, "/final", http.StatusFound)
			return
		}
		finalAuth.Store(r.Header.Get("Authorization"))
	})

	resp, err := c.OpenPrivateFile(srv.URL + "/start")
	if err != nil {
		t.Fatalf("OpenPrivateFile: %v", err)
	}
	resp.Body.Close()

	if finalAuth.Load() != "Bearer "+testToken {
		t.Errorf("Authorization after same-host redirect = %q", finalAuth.Load())
	}
}

func TestOpenPrivateFile_NonOKStatusIsAnError(t *testing.T) {
	c, srv := newFileServerClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})

	_, err := c.OpenPrivateFile(srv.URL + "/image.png")
	if err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("err = %v, want one naming status 403", err)
	}
}

func TestOpenPrivateFile_RetriesRateLimit(t *testing.T) {
	var calls atomic.Int32
	c, srv := newFileServerClient(t, func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		io.WriteString(w, "ok")
	})

	resp, err := c.OpenPrivateFile(srv.URL + "/image.png")
	if err != nil {
		t.Fatalf("OpenPrivateFile: %v", err)
	}
	resp.Body.Close()
	if calls.Load() != 2 {
		t.Errorf("calls = %d, want 2", calls.Load())
	}
}

func TestOpenPrivateFile_RateLimitExhaustedMapsToExit5(t *testing.T) {
	c, srv := newFileServerClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
	})
	c.MaxRetries = 1

	_, err := c.OpenPrivateFile(srv.URL + "/image.png")
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.ExitCode() != 5 {
		t.Fatalf("err = %v, want an APIError with exit code 5", err)
	}
}
