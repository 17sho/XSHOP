package botapi

import (
	"context"
	"errors"
	notifycontract "github.com/dujiao-next/internal/modules/telegram/notify/contract"
	"io"
	"net/http"
	"strings"
	"testing"
)

type failingTransport func(*http.Request) (*http.Response, error)

func (f failingTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestTelegramErrorBoundaryDoesNotExposeEncodedProviderDiagnostics(t *testing.T) {
	const token = "123456:superSecret"
	for _, message := range []string{"123456%3asuperSecret", "123456%253AsuperSecret", "provider echoed superSecret"} {
		c := NewWithHTTPClient(&http.Client{Transport: failingTransport(func(*http.Request) (*http.Response, error) { return nil, errors.New(message) })})
		err := c.SendWithBotToken(context.Background(), token, notifycontract.SendOptions{ChatID: "1", Message: "fixture"})
		if err == nil || strings.Contains(err.Error(), "superSecret") || !errors.Is(err, notifycontract.ErrNotifySendFailed) {
			t.Fatalf("provider diagnostic escaped redaction: %v", err)
		}
	}
}
func TestTelegramErrorBoundaryPreservesCancellationWithoutURL(t *testing.T) {
	c := NewWithHTTPClient(&http.Client{Transport: failingTransport(func(*http.Request) (*http.Response, error) { return nil, context.Canceled })})
	err := c.SendWithBotToken(context.Background(), "123456:superSecret", notifycontract.SendOptions{ChatID: "1", Message: "fixture"})
	if !errors.Is(err, context.Canceled) || !errors.Is(err, notifycontract.ErrNotifySendFailed) || strings.Contains(err.Error(), "superSecret") {
		t.Fatalf("unsafe or unclassified cancellation: %v", err)
	}
}

func TestAllTelegramErrorsRedactToken(t *testing.T) {
	const token = "123456:superSecret"
	for _, mode := range []string{"transport", "status", "description"} {
		t.Run(mode, func(t *testing.T) {
			c := NewWithHTTPClient(&http.Client{Transport: failingTransport(func(r *http.Request) (*http.Response, error) {
				if mode == "transport" {
					return nil, errors.New("dial failed " + r.URL.String())
				}
				status, body := 200, `{"ok":false,"description":"`+token+`"}`
				if mode == "status" {
					status = 500
					body = token
				}
				return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
			})})
			err := c.SendWithBotToken(context.Background(), token, notifycontract.SendOptions{ChatID: "1", Message: "test"})
			if err == nil || strings.Contains(err.Error(), token) || !errors.Is(err, notifycontract.ErrNotifySendFailed) {
				t.Fatalf("unsafe error: %v", err)
			}
		})
	}
}
