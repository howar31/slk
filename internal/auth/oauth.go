package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// TokenPair is the result of an OAuth exchange.
type TokenPair struct {
	UserToken string
	BotToken  string
}

// ExchangeCode trades an OAuth authorization code for tokens via oauth.v2.access.
// baseURL is normally https://slack.com/api (overridable for tests).
func ExchangeCode(baseURL, clientID, clientSecret, code, redirectURI string) (TokenPair, error) {
	form := url.Values{
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"code":          {code},
		"redirect_uri":  {redirectURI},
	}
	resp, err := http.PostForm(baseURL, form)
	if err != nil {
		return TokenPair{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	var r struct {
		OK          bool   `json:"ok"`
		Error       string `json:"error"`
		AccessToken string `json:"access_token"`
		AuthedUser  struct {
			AccessToken string `json:"access_token"`
		} `json:"authed_user"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return TokenPair{}, fmt.Errorf("oauth: invalid response: %w", err)
	}
	if !r.OK {
		return TokenPair{}, fmt.Errorf("oauth exchange failed: %s", r.Error)
	}
	return TokenPair{UserToken: r.AuthedUser.AccessToken, BotToken: r.AccessToken}, nil
}

// WaitForCode starts a local HTTP server on addr, returns the first OAuth
// "code" query parameter it receives, then shuts down. Times out after 5 min.
func WaitForCode(addr, callbackPath string) (string, error) {
	codeCh := make(chan string, 1)
	mux := http.NewServeMux()
	mux.HandleFunc(callbackPath, func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		if code == "" {
			http.Error(w, "missing code", http.StatusBadRequest)
			return
		}
		fmt.Fprintln(w, "slk: authorization received. You may close this tab.")
		codeCh <- code
	})
	srv := &http.Server{Addr: addr, Handler: mux}
	go srv.ListenAndServe()
	defer srv.Shutdown(context.Background())

	select {
	case code := <-codeCh:
		return code, nil
	case <-time.After(5 * time.Minute):
		return "", fmt.Errorf("oauth: timed out waiting for callback")
	}
}
