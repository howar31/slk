package api

import (
	"encoding/json"
	"fmt"
)

// CallAll repeatedly calls method, following response_metadata.next_cursor,
// until the cursor is empty or maxPages is reached. Returns one raw response
// per page.
func (c *Client) CallAll(method string, params map[string]string, maxPages int) ([][]byte, error) {
	// Copy the caller's map so we never mutate it.
	p := make(map[string]string, len(params))
	for k, v := range params {
		p[k] = v
	}

	var pages [][]byte
	cursor := ""
	for i := 0; i < maxPages; i++ {
		if cursor != "" {
			p["cursor"] = cursor
		}
		raw, err := c.Call(method, p, nil)
		if err != nil {
			return pages, err
		}
		pages = append(pages, raw)

		var meta struct {
			ResponseMetadata struct {
				NextCursor string `json:"next_cursor"`
			} `json:"response_metadata"`
		}
		if err := json.Unmarshal(raw, &meta); err != nil {
			return pages, fmt.Errorf("%s page %d: metadata unmarshal: %w", method, i+1, err)
		}
		cursor = meta.ResponseMetadata.NextCursor
		if cursor == "" {
			break
		}
	}
	return pages, nil
}
