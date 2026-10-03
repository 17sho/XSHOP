package xshopupgrade

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHelperPreparedStatusAndExplicitRestart(t *testing.T) {
	e, _, next := fixture(t)
	old, _ := FileHash(e.Target)
	candidate, _ := FileHash(next)
	if err := e.Prepare(context.Background(), next, old, candidate, 2, "xshop-production-b", strings.Repeat("a", 64)); err != nil {
		t.Fatal(err)
	}
	h := &Helper{Engine: e}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/status", nil))
	var s map[string]any
	json.Unmarshal(w.Body.Bytes(), &s)
	if s["state"] != "prepared" || s["need_restart"] != true {
		t.Fatal(w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/restart", nil))
	if w.Code != 202 {
		t.Fatal(w.Code, w.Body.String())
	}
	for deadline := time.Now().Add(time.Second); time.Now().Before(deadline); {
		h.mu.Lock()
		busy := h.busy
		state := h.status.State
		h.mu.Unlock()
		if !busy {
			if state != "installed" {
				t.Fatal(state)
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("restart did not finish")
}
