package upstream

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	siteconnectiondomain "github.com/dujiao-next/internal/modules/siteconnection/domain"
	"image"
	"image/png"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestImportedMediaValidation(t *testing.T) {
	var pngBody bytes.Buffer
	if err := png.Encode(&pngBody, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, body string
		valid      bool
	}{
		{"html", "<html>private service</html>", false},
		{"oversized", strings.Repeat("x", 10*1024*1024+1), false},
		{"truncated", "\x89PNG\r\n\x1a\n", false},
		{"valid", pngBody.String(), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			a := &DujiaoNextAdapter{uploadsDir: dir, client: &http.Client{Transport: diagnosticTransport(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(tc.body)), Header: make(http.Header)}, nil
			})}}
			path, err := a.DownloadImage(context.Background(), "https://media.example/file.html?token=private")
			if tc.valid {
				if err != nil || !strings.HasSuffix(path, ".png") {
					t.Fatalf("valid image: path=%q err=%v", path, err)
				}
			} else if err == nil {
				t.Fatal("unsafe image accepted")
			}
			files, _ := filepath.Glob(filepath.Join(dir, "upstream", "*"))
			if !tc.valid && len(files) != 0 {
				t.Fatalf("orphan files: %v", files)
			}
			if tc.valid {
				if _, err := os.Stat(filepath.Join(dir, strings.TrimPrefix(path, "/uploads/"))); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestImportedMediaRejectsPrivateTarget(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.Write([]byte("private")) }))
	defer srv.Close()
	a := NewDujiaoNextAdapter(&siteconnectiondomain.Connection{BaseURL: srv.URL}, t.TempDir())
	_, err := a.DownloadImage(context.Background(), srv.URL+"/secret")
	if err == nil || calls != 0 {
		t.Fatalf("private target reached: calls=%d err=%v", calls, err)
	}
}

func TestImportedMediaNetworkPolicy(t *testing.T) {
	for _, addr := range []string{"127.0.0.1", "10.0.0.1", "169.254.169.254", "100.64.0.1", "192.0.2.1", "::1", "::ffff:127.0.0.1", "fc00::1", "fe80::1", "64:ff9b::a00:1", "64:ff9b:1::a00:1", "2001::1", "2001:db8::1", "2002:7f00:1::", "3fff::1"} {
		if publicImageIP(net.ParseIP(addr)) {
			t.Errorf("nonpublic accepted: %s", addr)
		}
	}
	for _, addr := range []string{"8.8.8.8", "2606:4700:4700::1111"} {
		if !publicImageIP(net.ParseIP(addr)) {
			t.Errorf("public rejected: %s", addr)
		}
	}
	c := newImageHTTPClient()
	if c.Transport.(*http.Transport).Proxy != nil {
		t.Fatal("media uses environment proxy")
	}
	calls := 0
	c.Transport = diagnosticTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 302, Header: http.Header{"Location": []string{"http://127.0.0.1/secret"}}, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
	})
	a := &DujiaoNextAdapter{uploadsDir: t.TempDir(), imageClient: c}
	if _, err := a.DownloadImage(context.Background(), "https://media.example/image"); err == nil || calls != 1 {
		t.Fatal("redirect followed or accepted")
	}
}

type diagnosticTransport func(*http.Request) (*http.Response, error)

func (f diagnosticTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestUpstreamRequestDiagnosticsHideURLs(t *testing.T) {
	for _, operation := range []string{"ping", "get-product", "get-order", "create-order", "download"} {
		for _, failure := range []string{"construction", "transport", "canceled", "deadline"} {
			t.Run(operation+"/"+failure, func(t *testing.T) {
				const marker = "PRIVATE-URL-MARKER"
				baseURL := "https://fixture.invalid/?token=" + marker
				ctx := context.Background()
				var wantCause error
				switch failure {
				case "construction":
					baseURL += "\n"
				case "canceled":
					var cancel context.CancelFunc
					ctx, cancel = context.WithCancel(ctx)
					cancel()
					wantCause = context.Canceled
				case "deadline":
					var cancel context.CancelFunc
					ctx, cancel = context.WithDeadline(ctx, time.Unix(1, 0))
					defer cancel()
					wantCause = context.DeadlineExceeded
				}
				a := &DujiaoNextAdapter{baseURL: baseURL, client: &http.Client{Transport: diagnosticTransport(func(r *http.Request) (*http.Response, error) {
					if err := r.Context().Err(); err != nil {
						return nil, err
					}
					return nil, fmt.Errorf("transport echoed %s", r.URL)
				})}}
				var err error
				switch operation {
				case "ping":
					_, err = a.Ping(ctx)
				case "get-product":
					_, err = a.GetProduct(ctx, 42)
				case "get-order":
					_, err = a.GetOrder(ctx, 42)
				case "create-order":
					_, err = a.CreateOrder(ctx, CreateUpstreamOrderReq{})
				case "download":
					_, err = a.DownloadImage(ctx, baseURL)
				}
				if err == nil {
					t.Fatal("expected request failure")
				}
				for cause := err; cause != nil; cause = errors.Unwrap(cause) {
					if strings.Contains(cause.Error(), marker) || strings.Contains(cause.Error(), "fixture.invalid") {
						t.Errorf("URL leaked in diagnostic: %v", cause)
					}
				}
				if wantCause != nil && !errors.Is(err, wantCause) {
					t.Errorf("lost cancellation classification: %v", err)
				}
			})
		}
	}
}

func TestUpstreamHTTPErrorClassificationSurvivesRedaction(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error_code":"product_deleted","error_message":"PRIVATE-CARD-MARKER"}`))
	}))
	defer srv.Close()
	a := &DujiaoNextAdapter{baseURL: srv.URL, client: srv.Client()}
	_, err := a.Ping(context.Background())
	var statusErr *upstreamHTTPError
	if !errors.As(err, &statusErr) || statusErr.Status != http.StatusNotFound || extractUpstreamErrorCode(err) != "product_deleted" {
		t.Fatalf("lost HTTP classification: %v", err)
	}
	if strings.Contains(err.Error(), "PRIVATE-CARD-MARKER") {
		t.Fatal("remote payload exposed")
	}
	if _, err := a.GetProduct(context.Background(), 42); !errors.Is(err, ErrUpstreamProductDeleted) {
		t.Fatalf("lost product sentinel: %v", err)
	}
	if _, err := a.DownloadImage(context.Background(), srv.URL+"/image"); err == nil || err.Error() != "download image: status 404" {
		t.Fatalf("lost image HTTP status: %v", err)
	}
}

func TestUpstreamErrorsDoNotExposeRemotePayloads(t *testing.T) {
	for _, e := range []*upstreamHTTPError{{Status: 500, Body: "PRIVATE-CARD-MARKER"}, {Status: 400, Code: "bad_request", Message: "PRIVATE-CARD-MARKER", Body: "PRIVATE-CARD-MARKER"}, {Status: 400, Code: "PRIVATE-CARD-MARKER\n"}} {
		if strings.Contains(e.Error(), "PRIVATE-CARD-MARKER") {
			t.Fatal("remote response data leaked in error string")
		}
		if !strings.Contains(e.Error(), "status") {
			t.Fatal("diagnostic status lost")
		}
	}
	e := &upstreamHTTPError{Status: 400, Code: "insufficient_balance"}
	if extractUpstreamErrorCode(e) != "insufficient_balance" {
		t.Fatal("machine error classification lost")
	}
}
