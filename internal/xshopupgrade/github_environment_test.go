package xshopupgrade

import (
	"context"
	"slices"
	"strings"
	"testing"
)

func TestGitHubCommandPinsHostAndRejectsInheritedAuthorityEnvironment(t *testing.T) {
	t.Setenv("GH_HOST", "evil.invalid")
	t.Setenv("GH_REPO", "other/other")
	t.Setenv("GH_TOKEN", "synthetic-no-network-fixture")
	t.Setenv("GH_CONFIG_DIR", "/tmp/untrusted-fixture")
	cmd := githubCommand(context.Background(), "api", "repos/"+Repository+"/releases/latest")
	if cmd.Path != "/usr/bin/gh" || !slices.Equal(cmd.Args[1:4], []string{"api", "--hostname", "github.com"}) {
		t.Fatalf("host/binary not pinned: %v", cmd.Args)
	}
	env := strings.Join(cmd.Env, "\n")
	for _, bad := range []string{"evil.invalid", "other/other", "synthetic-no-network-fixture", "/tmp/untrusted-fixture"} {
		if strings.Contains(env, bad) {
			t.Fatal("inherited host/credential authority")
		}
	}
	for _, good := range []string{"HOME=/root", "GH_CONFIG_DIR=/root/.config/gh", "GH_PROMPT_DISABLED=1"} {
		if !strings.Contains(env, good) {
			t.Fatalf("required fixed backend setting missing: %s", good)
		}
	}
}
