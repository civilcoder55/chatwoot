package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"gateway/internal/call"
	"gateway/internal/config"
	"gateway/internal/tenant"
	"gateway/internal/webhook"
)

func newTestHandler(t *testing.T) (http.Handler, *call.Registry, *tenant.Store) {
	t.Helper()

	dir := t.TempDir()
	tenantsPath := filepath.Join(dir, "tenants.yaml")
	os.WriteFile(tenantsPath, []byte(`tenants:
  - phone_number: "+1111111111"
    key: "test-key"
`), 0o644)

	store, err := tenant.LoadStore(tenantsPath)
	if err != nil {
		t.Fatalf("LoadStore: %v", err)
	}

	registry := call.NewRegistry()
	cfg := &config.Config{
		STUNServer:  "stun:stun.l.google.com:19302",
		RecordingDir: dir,
	}
	wh := webhook.NewClient("http://127.0.0.1:1/events", "secret")

	// Pass nil for SIP server since we only test auth/routing/validation
	handler := NewHandler(cfg, registry, nil, wh, store)
	return handler, registry, store
}

func TestHealthEndpoint(t *testing.T) {
	handler, _, _ := newTestHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("health status = %d, want %d", w.Code, http.StatusOK)
	}

	var body map[string]any
	json.NewDecoder(w.Body).Decode(&body)
	if body["status"] != "ok" {
		t.Errorf("status = %v, want ok", body["status"])
	}
}

func TestAuthenticateMiddleware_Unauthorized(t *testing.T) {
	handler, _, _ := newTestHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/validate", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestAuthenticateMiddleware_ValidCredentials(t *testing.T) {
	handler, _, _ := newTestHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/validate", nil)
	req.Header.Set("X-Phone-Number", "+1111111111")
	req.Header.Set("X-Api-Key", "test-key")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestInitiateCall_MissingFields(t *testing.T) {
	handler, _, _ := newTestHandler(t)

	body, _ := json.Marshal(map[string]string{"to": "+2222222222"})
	req := httptest.NewRequest(http.MethodPost, "/calls/initiate", bytes.NewReader(body))
	req.Header.Set("X-Phone-Number", "+1111111111")
	req.Header.Set("X-Api-Key", "test-key")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestInitiateCall_ForbiddenFrom(t *testing.T) {
	handler, _, _ := newTestHandler(t)

	body, _ := json.Marshal(map[string]string{
		"to":        "+2222222222",
		"from":      "+9999999999",
		"sdp_offer": "v=0...",
	})
	req := httptest.NewRequest(http.MethodPost, "/calls/initiate", bytes.NewReader(body))
	req.Header.Set("X-Phone-Number", "+1111111111")
	req.Header.Set("X-Api-Key", "test-key")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestAcceptCall_NotFound(t *testing.T) {
	handler, _, _ := newTestHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/calls/nonexistent/accept", nil)
	req.Header.Set("X-Phone-Number", "+1111111111")
	req.Header.Set("X-Api-Key", "test-key")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestRejectCall_NotFound(t *testing.T) {
	handler, _, _ := newTestHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/calls/nonexistent/reject", nil)
	req.Header.Set("X-Phone-Number", "+1111111111")
	req.Header.Set("X-Api-Key", "test-key")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestTerminateCall_NotFound(t *testing.T) {
	handler, _, _ := newTestHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/calls/nonexistent/terminate", nil)
	req.Header.Set("X-Phone-Number", "+1111111111")
	req.Header.Set("X-Api-Key", "test-key")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestResolveSession_Forbidden(t *testing.T) {
	handler, registry, _ := newTestHandler(t)

	session := &call.Session{
		CallID:    "call-owned-by-other",
		Direction: call.DirectionInbound,
		To:        "+9999999999",
		From:      "+2222222222",
		Status:    call.StatusRinging,
	}
	registry.Add(session)

	req := httptest.NewRequest(http.MethodPost, "/calls/call-owned-by-other/reject", nil)
	req.Header.Set("X-Phone-Number", "+1111111111")
	req.Header.Set("X-Api-Key", "test-key")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestAcceptCall_Forbidden(t *testing.T) {
	handler, registry, _ := newTestHandler(t)

	session := &call.Session{
		CallID:    "call-123",
		Direction: call.DirectionInbound,
		To:        "+9999999999",
		From:      "+2222222222",
		Status:    call.StatusRinging,
	}
	registry.Add(session)

	body, _ := json.Marshal(map[string]string{"sdp_answer": "v=0..."})
	req := httptest.NewRequest(http.MethodPost, "/calls/call-123/accept", bytes.NewReader(body))
	req.Header.Set("X-Phone-Number", "+1111111111")
	req.Header.Set("X-Api-Key", "test-key")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSON(w, http.StatusCreated, map[string]string{"key": "value"})

	if w.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", w.Code, http.StatusCreated)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var body map[string]string
	json.NewDecoder(w.Body).Decode(&body)
	if body["key"] != "value" {
		t.Errorf("body[key] = %q, want value", body["key"])
	}
}
