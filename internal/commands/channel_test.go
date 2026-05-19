package commands

import (
	"bytes"
	"strings"
	"testing"
)

func TestChannelWrite_DryRun(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"create", "--name", "new-chan"}, "conversations.create"},
		{[]string{"archive", "--channel", "C1"}, "conversations.archive"},
		{[]string{"invite", "--channel", "C1", "--users", "U1,U2"}, "conversations.invite"},
		{[]string{"topic", "--channel", "C1", "--topic", "hello"}, "conversations.setTopic"},
	} {
		g := &GlobalFlags{DryRun: true}
		cmd := newChannelCommand(g)
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetArgs(tc.args)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("execute %v: %v", tc.args, err)
		}
		if !strings.Contains(out.String(), tc.want) {
			t.Fatalf("want %q in %q", tc.want, out.String())
		}
	}
}
