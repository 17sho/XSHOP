package main

import (
	"github.com/dujiao-next/internal/xshopupgrade"
	"os"
	"path/filepath"
	"testing"
)

func TestBootstrapRequiresExplicitRootCLIAndNeverResetsExistingFence(t *testing.T) {
	root := t.TempDir()
	e := &xshopupgrade.Engine{StateDir: root}
	if _, err := bootstrap(e, nil); err == nil {
		t.Fatal("daemon silently reset missing fence")
	}
	if done, err := bootstrap(e, []string{"--initialize"}); err != nil || !done {
		t.Fatal(err)
	}
	if _, err := bootstrap(e, []string{"--initialize"}); err == nil {
		t.Fatal("existing state reset")
	}
	if _, err := bootstrap(e, []string{"--other"}); err == nil {
		t.Fatal("arbitrary CLI accepted")
	}
	if _, err := bootstrap(e, nil); err != nil {
		t.Fatal(err)
	}
	os.Remove(filepath.Join(root, "state.json"))
	if _, err := bootstrap(e, nil); err == nil {
		t.Fatal("lost trusted state reset")
	}
}
