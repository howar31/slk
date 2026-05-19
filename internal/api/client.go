package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client is a thin Slack Web API HTTP client.
type Client struct {
	Token      string
	BaseURL    string // overridable for tests; default https://slack.com/api
	HTTP       *http.Client
	MaxRetries int // rate-limit retries
}

// New returns a Client for the given token.
func New(token string) *Client {
	return &Client{
		Token:      token,
		BaseURL:    "https://slack.com/api",
		HTTP:       &http.Client{Timeout: 30 * time.Second},
		MaxRetries: 3,
	}
}

// Call invokes a Slack method. params become POST form fields; if body is
// non-nil it is sent as a JSON body instead. Returns the raw response bytes.
func (c *Client) Call(method string, params map[string]string, body []byte) ([]byte, error) {
	endpoint := c.BaseURL + "/" + method

	for attempt := 0; ; attempt++ {
		req, err := c.buildRequest(endpoint, params, body)
		if err != nil {
			return nil, err
		}
		resp, err := c.HTTP.Do(req)
		if err != nil {
			return nil, err
		}
		raw, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("%s: reading response body: %w", method, err)
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			if attempt < c.MaxRetries {
				time.Sleep(parseRetryAfter(resp.Header.Get("Retry-After")))
				continue
			}
			return nil, &APIError{Method: method, SlackError: "ratelimited"}
		}

		var envelope struct {
			OK    bool   `json:"ok"`
			Error string `json:"error"`
		}
		if err := json.Unmarshal(raw, &envelope); err != nil {
			return nil, fmt.Errorf("%s: invalid JSON response: %w", method, err)
		}
		if !envelope.OK {
			if envelope.Error == "ratelimited" && attempt < c.MaxRetries {
				time.Sleep(time.Second * time.Duration(attempt+1))
				continue
			}
			return nil, &APIError{Method: method, SlackError: envelope.Error}
		}
		return raw, nil
	}
}

func (c *Client) buildRequest(endpoint string, params map[string]string, body []byte) (*http.Request, error) {
	var req *http.Request
	var err error
	if body != nil {
		req, err = http.NewRequest("POST", endpoint, bytes.NewReader(body))
		if err == nil {
			req.Header.Set("Content-Type", "application/json; charset=utf-8")
		}
	} else {
		form := url.Values{}
		for k, v := range params {
			form.Set(k, v)
		}
		req, err = http.NewRequest("POST", endpoint, strings.NewReader(form.Encode()))
		if err == nil {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
	}
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	return req, nil
}

func parseRetryAfter(h string) time.Duration {
	var secs int
	if _, err := fmt.Sscanf(h, "%d", &secs); err == nil && secs > 0 {
		return time.Duration(secs) * time.Second
	}
	return time.Second
}
