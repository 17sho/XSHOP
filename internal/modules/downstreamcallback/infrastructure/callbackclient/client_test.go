package callbackclient

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	downstreamcontract "github.com/dujiao-next/internal/modules/downstreamcallback/contract"
	"github.com/dujiao-next/internal/shared/jsonmap"
	"github.com/dujiao-next/internal/upstream"
)

func TestClientSendsSignedCallback(t *testing.T) {
	const secret = "downstream-secret"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read callback body: %v", err)
			return
		}
		if request.Method != http.MethodPost || request.Header.Get(upstream.HeaderApiKey) != "downstream-key" {
			t.Errorf("unexpected request: method=%s api_key=%q", request.Method, request.Header.Get(upstream.HeaderApiKey))
		}
		timestamp, err := strconv.ParseInt(request.Header.Get(upstream.HeaderTimestamp), 10, 64)
		if err != nil {
			t.Errorf("parse timestamp: %v", err)
			return
		}
		wantSignature := upstream.Sign(secret, http.MethodPost, signaturePath, timestamp, body)
		if request.Header.Get(upstream.HeaderSignature) != wantSignature {
			t.Errorf("signature mismatch: got=%q want=%q", request.Header.Get(upstream.HeaderSignature), wantSignature)
		}
		var payload downstreamcontract.CallbackPayload
		if err := json.Unmarshal(body, &payload); err != nil || payload.Event != "order.fulfilled" {
			t.Errorf("payload mismatch: payload=%#v err=%v", payload, err)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(server.Close)

	client := NewWithHTTPClient(server.Client())
	err := client.Send(context.Background(), downstreamcontract.DeliveryRequest{
		URL:       server.URL,
		APIKey:    "downstream-key",
		APISecret: secret,
		Payload: downstreamcontract.CallbackPayload{
			Event:     "order.fulfilled",
			OrderID:   8,
			Timestamp: 1_700_000_000,
		},
	})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
}

func TestClientRejectsNonSuccessContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusBadGateway)
		_, _ = writer.Write([]byte("upstream unavailable"))
	}))
	t.Cleanup(server.Close)

	err := New().Send(context.Background(), downstreamcontract.DeliveryRequest{
		URL:     server.URL,
		Payload: downstreamcontract.CallbackPayload{Timestamp: 1_700_000_000},
	})
	if err == nil {
		t.Fatal("Send() should reject non-success response")
	}
}

func TestDefaultClientRejectsLoopbackTarget(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		t.Errorf("loopback callback target must not be reached")
		_, _ = writer.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(server.Close)

	err := New().Send(context.Background(), downstreamcontract.DeliveryRequest{
		URL:     server.URL,
		APIKey:  "downstream-key",
		Payload: downstreamcontract.CallbackPayload{Event: "order.fulfilled", Timestamp: 1_700_000_000},
	})
	if err == nil {
		t.Fatalf("Send() to loopback target should fail")
	}
}

func TestIsPublicIP(t *testing.T) {
	for _, tc := range []struct {
		ip   string
		want bool
	}{
		{"127.0.0.1", false}, {"10.1.2.3", false}, {"192.168.1.1", false}, {"172.16.0.9", false},
		{"169.254.169.254", false}, {"100.64.0.1", false}, {"0.0.0.0", false}, {"::1", false}, {"fd00::1", false},
		{"8.8.8.8", true}, {"1.1.1.1", true}, {"2606:4700::1111", true},
	} {
		if got := isPublicIP(net.ParseIP(tc.ip)); got != tc.want {
			t.Errorf("isPublicIP(%s) = %v, want %v", tc.ip, got, tc.want)
		}
	}
}

func TestCallbackWireContractAndQueryRemainUnchanged(t *testing.T) {
	const body = `{"event":"order.fulfilled","order_id":8,"order_no":"DJ-8","downstream_order_no":"remote-8","status":"delivered","fulfillment":{"type":"auto","status":"delivered","payload":"FIXTURE-CARD","delivery_data":{"tracking_no":"FIXTURE-TRACK"},"delivered_at":"2023-11-14T22:13:20Z"},"timestamp":1700000000}`
	const secret = "fixture-secret"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		actual, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		if string(actual) != body {
			t.Errorf("wire contract changed: %s", actual)
		}
		if r.URL.RawQuery != "token=FIXTURE-QUERY" {
			t.Error("destination query modified")
		}
		want := upstream.Sign(secret, http.MethodPost, "/api/v1/upstream/callback", 1700000000, []byte(body))
		if r.Header.Get(upstream.HeaderSignature) != want {
			t.Error("signature algorithm/path changed")
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	at := time.Unix(1700000000, 0).UTC()
	err := NewWithHTTPClient(server.Client()).Send(context.Background(), downstreamcontract.DeliveryRequest{
		URL: server.URL + "/custom-path?token=FIXTURE-QUERY", APIKey: "fixture-key", APISecret: secret,
		Payload: downstreamcontract.CallbackPayload{Event: "order.fulfilled", OrderID: 8, OrderNo: "DJ-8", DownstreamOrderNo: "remote-8", Status: "delivered", Timestamp: 1700000000,
			Fulfillment: &downstreamcontract.Fulfillment{Type: "auto", Status: "delivered", Payload: "FIXTURE-CARD", DeliveryData: jsonmap.JSON{"tracking_no": "FIXTURE-TRACK"}, DeliveredAt: &at}},
	})
	if err != nil {
		t.Fatal(err)
	}
}

type failingTransport struct{}

func (failingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("reflected FIXTURE-CARD")
}

func TestCallbackErrorsDoNotReflectSecrets(t *testing.T) {
	for _, status := range []int{200, 502} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
			_, _ = w.Write([]byte("FIXTURE-CARD FIXTURE-QUERY"))
		}))
		err := NewWithHTTPClient(server.Client()).Send(context.Background(), downstreamcontract.DeliveryRequest{URL: server.URL + "?token=FIXTURE-QUERY"})
		server.Close()
		if err == nil || strings.Contains(err.Error(), "FIXTURE-") {
			t.Fatalf("response secrets in error: %v", err)
		}
	}
	for _, url := range []string{"https://callback.example.test?token=FIXTURE-QUERY", "https://bad%FIXTURE-QUERY"} {
		err := NewWithHTTPClient(&http.Client{Transport: failingTransport{}}).Send(context.Background(), downstreamcontract.DeliveryRequest{URL: url})
		if err == nil || strings.Contains(err.Error(), "FIXTURE-") {
			t.Fatalf("transport/URL secrets in error: %v", err)
		}
	}
}
