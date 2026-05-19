package api

import "encoding/json"

// CallAll repeatedly calls method, following response_metadata.next_cursor,
// until the cursor is empty or maxPages is reached. Returns one raw response
// per page.
func (c *Client) CallAll(method string, params map[string]string, maxPages int) ([][]byte, error) {
	if params == nil {
		params = map[string]string{}
	}
	var pages [][]byte
	cursor := ""
	for i := 0; i < maxPages; i++ {
		if cursor != "" {
			params["cursor"] = cursor
		}
		raw, err := c.Call(method, params, nil)
		if err != nil {
			return pages, err
		}
		pages = append(pages, raw)

		var meta struct {
			ResponseMetadata struct {
				NextCursor string `json:"next_cursor"`
			} `json:"response_metadata"`
		}
		json.Unmarshal(raw, &meta)
		cursor = meta.ResponseMetadata.NextCursor
		if cursor == "" {
			break
		}
	}
	return pages, nil
}
