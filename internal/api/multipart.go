package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
)

// CallMultipart invokes a Slack method with a multipart/form-data body carrying
// text fields plus a single file part. Used by methods that take a binary upload
// (e.g. users.setPhoto with the `image` field). Returns the raw response bytes.
func (c *Client) CallMultipart(method string, fields map[string]string, fileField, filename string, data []byte) ([]byte, error) {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	for k, v := range fields {
		if err := w.WriteField(k, v); err != nil {
			return nil, err
		}
	}
	part, err := w.CreateFormFile(fileField, filename)
	if err != nil {
		return nil, err
	}
	if _, err := part.Write(data); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", c.BaseURL+"/"+method, &body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+c.Token)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	raw, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("%s: reading response body: %w", method, err)
	}

	var envelope struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("%s: invalid JSON response: %w", method, err)
	}
	if !envelope.OK {
		return nil, &APIError{Method: method, SlackError: envelope.Error}
	}
	return raw, nil
}
