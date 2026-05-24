package commands

import (
	"reflect"
	"testing"
)

func TestScopeUnion_Oracle(t *testing.T) {
	root := NewRootCommand("test")

	wantUser := []string{
		"bookmarks:read", "bookmarks:write", "canvases:read", "canvases:write",
		"channels:history", "channels:read", "channels:write", "chat:write",
		"dnd:read", "dnd:write", "emoji:read", "files:read", "files:write",
		"groups:history", "groups:read", "groups:write", "im:history", "im:read",
		"im:write", "lists:read", "lists:write", "mpim:history", "mpim:read",
		"mpim:write", "pins:read", "pins:write", "reactions:read", "reactions:write",
		"search:read", "team:read", "usergroups:read", "usergroups:write",
		"users.profile:read", "users.profile:write", "users:read", "users:read.email",
		"users:write",
	}
	wantBot := []string{
		"bookmarks:read", "bookmarks:write", "canvases:read", "canvases:write",
		"channels:history", "channels:join", "channels:manage", "channels:read",
		"chat:write", "dnd:read", "emoji:read", "files:read", "files:write",
		"groups:history", "groups:read", "groups:write", "im:history", "im:read",
		"im:write", "lists:read", "lists:write", "mpim:history", "mpim:read",
		"mpim:write", "pins:read", "pins:write", "reactions:read", "reactions:write",
		"team:read", "usergroups:read", "usergroups:write", "users.profile:read",
		"users:read", "users:read.email", "users:write",
	}

	if got := scopeUnion(root, "user"); !reflect.DeepEqual(got, wantUser) {
		t.Errorf("user scopes (%d):\n got  %v\n want %v", len(got), got, wantUser)
	}
	if got := scopeUnion(root, "bot"); !reflect.DeepEqual(got, wantBot) {
		t.Errorf("bot scopes (%d):\n got  %v\n want %v", len(got), got, wantBot)
	}
}
