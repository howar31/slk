package commands

import (
	"encoding/json"
	"path/filepath"

	"github.com/howar31/slk/internal/api"
	"github.com/howar31/slk/internal/resolve"
)

// slackLookup resolves user IDs to display names via users.info.
type slackLookup struct{ client *api.Client }

func (l slackLookup) Name(id string) (string, error) {
	raw, err := l.client.Call("users.info", map[string]string{"user": id}, nil)
	if err != nil {
		return "", err
	}
	var resp struct {
		User struct {
			Name     string `json:"name"`
			RealName string `json:"real_name"`
		} `json:"user"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", err
	}
	if resp.User.RealName != "" {
		return resp.User.RealName, nil
	}
	return resp.User.Name, nil
}

// newResolver returns an ID resolver, or nil if --no-resolve is set.
func newResolver(g *GlobalFlags, client *api.Client) *resolve.Resolver {
	if g.NoResolve {
		return nil
	}
	return resolve.New(filepath.Join(cacheDir(), "names.json"), slackLookup{client})
}

// resolveUser applies r to id; a nil r (or empty id) returns id unchanged.
func resolveUser(r *resolve.Resolver, id string) string {
	if r == nil || id == "" {
		return id
	}
	return r.Resolve(id)
}
