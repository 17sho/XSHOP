//go:build xshop_production

package xshopupgrade

import (
	"strings"
	"testing"
)

func TestProductionCompileBoundary(t *testing.T) {
	if PreviewService != "dujiao-next.service" || PreviewRoot != "/opt/dujiao-next" || StateRoot != "/var/lib/xshop-production-upgrader" || SocketPath != "/run/xshop-production-upgrader/control.sock" {
		t.Fatal("production namespace not fixed")
	}
	if !ValidTag("xshop-production-v1") || ValidTag("xshop-preview-v1") {
		t.Fatal("cross tag namespace")
	}
	if !PeerAllowed(996, 996, "0::/system.slice/dujiao-next.service\n") || PeerAllowed(996, 996, "0::/system.slice/dujiao-preview.service\n") {
		t.Fatal("cross peer")
	}
	if !strings.Contains(previewHashWorker, "0::/system.slice/dujiao-next.service") || strings.Contains(previewHashWorker, "dujiao-preview.service") {
		t.Fatal("worker cross scope")
	}
}
