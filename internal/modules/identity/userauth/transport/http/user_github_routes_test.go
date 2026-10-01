package userauthhttp

import (
	"os"
	"strings"
	"testing"
)

func TestGitHubOAuthStartDoesNotUseLoginAttemptRateLimiter(t *testing.T) {
	source, err := os.ReadFile("user_github_handler.go")
	if err != nil {
		t.Fatalf("read GitHub auth routes: %v", err)
	}
	if strings.Contains(string(source), `auth.GET("/github", limit, h.Start)`) {
		t.Fatal("opening a GitHub OAuth popup must not consume login attempt limits")
	}
	if !strings.Contains(string(source), `auth.GET("/github", h.Start)`) {
		t.Fatal("GitHub OAuth start route is not registered")
	}
}
