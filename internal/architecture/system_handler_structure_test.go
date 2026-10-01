package architecture

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSystemHTTPLivesInTransport(t *testing.T) {
	repositoryRoot := findRepositoryRoot(t)
	transportRoot := filepath.Join(repositoryRoot, "internal", "platform", "http", "system")

	if entries, err := os.ReadDir(transportRoot); err == nil && len(entries) > 0 {
		t.Fatalf("removed system update transport must stay empty: %s", transportRoot)
	} else if err != nil && !os.IsNotExist(err) {
		t.Fatalf("read removed system update transport: %v", err)
	}

	legacy := filepath.Join(repositoryRoot, "internal", "http", "handlers", "admin", "admin_system.go")
	if _, err := os.Stat(legacy); err == nil {
		t.Fatalf("legacy system handler must stay removed: %s", legacy)
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat legacy system handler: %v", err)
	}
}
