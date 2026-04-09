package webhook

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientSend(t *testing.T) {
	var received map[string]any

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get(HeaderGatewaySecret); got != "test-secret" {
			t.Errorf("secret header = %q, want %q", got, "test-secret")
		}
		if got := r.Header.Get("Content-Type"); got != contentTypeJSON {
			t.Errorf("Content-Type = %q, want %q", got, contentTypeJSON)
		}
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}

		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Errorf("decode body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	c := NewClient(ts.URL, "test-secret")
	err := c.Send("call.ringing", map[string]any{"call_id": "abc"})
	if err != nil {
		t.Fatalf("Send() error: %v", err)
	}

	if received["event"] != "call.ringing" {
		t.Errorf("event = %v, want %q", received["event"], "call.ringing")
	}

	data, ok := received["data"].(map[string]any)
	if !ok {
		t.Fatal("data field missing or wrong type")
	}
	if data["call_id"] != "abc" {
		t.Errorf("data.call_id = %v, want %q", data["call_id"], "abc")
	}
}

func TestClientSendErrorStatus(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	c := NewClient(ts.URL, "secret")
	err := c.Send("call.test", map[string]any{})
	if err == nil {
		t.Error("expected error for 500 response")
	}
}

func TestClientSendNetworkError(t *testing.T) {
	c := NewClient("http://127.0.0.1:1", "secret")
	err := c.Send("call.test", map[string]any{})
	if err == nil {
		t.Error("expected error for network failure")
	}
}

func TestRecordingURL(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected string
	}{
		{"events suffix", "http://localhost:3000/webhooks/sip_gateway/events", "http://localhost:3000/webhooks/sip_gateway/recordings"},
		{"no events suffix", "http://localhost:3000/webhooks/sip_gateway", "http://localhost:3000/webhooks/sip_gateway/recordings"},
		{"trailing slash", "http://localhost:3000/webhooks/events/", "http://localhost:3000/webhooks/recordings"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewClient(tt.url, "secret")
			if got := c.recordingURL(); got != tt.expected {
				t.Errorf("recordingURL() = %q, want %q", got, tt.expected)
			}
		})
	}
}
