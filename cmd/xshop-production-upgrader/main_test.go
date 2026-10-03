package main

import (
	"github.com/dujiao-next/internal/xshopupgrade"
	"strings"
	"testing"
)

func TestDedicatedProductionCommandRejectsDefaultBuild(t *testing.T) {
	if !xshopupgrade.Production {
		if err := run(); err == nil || !strings.Contains(err.Error(), "production build tag required") {
			t.Fatal(err)
		}
	}
}
