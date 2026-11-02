package api

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"time"
)

// downloadTimeout bounds one file download, body included. It is separate from
// the Web API client's timeout, which is too short for large files.
const downloadTimeout = 10 * time.Minute

// maxFileRedirects matches net/http's default redirect limit, which a custom
// CheckRedirect replaces.
const maxFileRedirects = 10

// checkFileURL reports whether the token may be sent to rawURL: HTTPS, no
// userinfo, and a host exactly equal to an entry in FileHosts. The URL is left
// out of errors so that nothing sensitive in it reaches logs.
func (c *Client) checkFileURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return errors.New("invalid file URL")
	}
	if u.Scheme != "https" || u.User != nil {
		return errors.New("refusing to send credentials: file URL is not plain HTTPS")
	}
	if !slices.Contains(c.FileHosts, u.Hostname()) {
		return fmt.Errorf("refusing to send credentials to host %q: not a Slack file host", u.Hostname())
	}
	return nil
}

// OpenPrivateFile GETs a Slack-hosted file (url_private or
// url_private_download) with the token. Remote files expose app-supplied URLs
// on any host, so the URL is checked before the token is attached, and a
// redirect outside FileHosts is followed without credentials. The caller owns
// the returned body.
func (c *Client) OpenPrivateFile(rawURL string) (*http.Response, error) {
	if err := c.checkFileURL(rawURL); err != nil {
		return nil, err
	}
	httpClient := &http.Client{
		Transport: c.HTTP.Transport,
		Timeout:   downloadTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxFileRedirects {
				return fmt.Errorf("stopped after %d redirects", maxFileRedirects)
			}
			if c.checkFileURL(req.URL.String()) != nil {
				req.Header.Del("Authorization")
				req.Header.Del("Cookie")
			}
			return nil
		},
	}

	for attempt := 0; ; attempt++ {
		req, err := http.NewRequest(http.MethodGet, rawURL, nil)
		if err != nil {
			return nil, errors.New("invalid file URL")
		}
		req.Header.Set("Authorization", "Bearer "+c.Token)
		resp, err := httpClient.Do(req)
		if err != nil {
			var urlErr *url.Error
			if errors.As(err, &urlErr) {
				return nil, fmt.Errorf("downloading file: %w", urlErr.Err)
			}
			return nil, fmt.Errorf("downloading file: %w", err)
		}
		switch resp.StatusCode {
		case http.StatusOK:
			return resp, nil
		case http.StatusTooManyRequests:
			resp.Body.Close()
			if attempt >= c.MaxRetries {
				return nil, &APIError{Method: "file download", SlackError: "ratelimited"}
			}
			time.Sleep(parseRetryAfter(resp.Header.Get("Retry-After")))
		default:
			resp.Body.Close()
			return nil, fmt.Errorf("file download returned status %d", resp.StatusCode)
		}
	}
}
