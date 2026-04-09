// Package api provides the HTTP API for managing SIP gateway calls.
package api

import (
	"encoding/json"
	"net/http"

	"gateway/internal/bridge"
	"gateway/internal/call"
	"gateway/internal/config"
	"gateway/internal/services"
	sipserver "gateway/internal/sip"
	"gateway/internal/tenant"
	"gateway/internal/webhook"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

const (
	HeaderPhoneNumber = "X-Phone-Number"
	HeaderApiKey      = "X-Api-Key"
)

type Handler struct {
	cfg      *config.Config
	registry *call.Registry
	sip      *sipserver.Server
	webhook  *webhook.Client
	tenants  *tenant.Store
	mux      *http.ServeMux
	cs       *services.CallService
}

// NewHandler creates a new HTTP handler with all routes registered.
func NewHandler(cfg *config.Config, registry *call.Registry, sip *sipserver.Server, wh *webhook.Client, ts *tenant.Store) http.Handler {
	cs := services.NewService(registry, sip, wh, cfg)

	h := &Handler{
		cfg:      cfg,
		registry: registry,
		sip:      sip,
		webhook:  wh,
		tenants:  ts,
		mux:      http.NewServeMux(),
		cs:       cs,
	}

	h.mux.HandleFunc("POST /calls/initiate", h.authenticate(h.initiateCall))
	h.mux.HandleFunc("POST /calls/{callID}/accept", h.authenticate(h.acceptCall))
	h.mux.HandleFunc("POST /calls/{callID}/reject", h.authenticate(h.rejectCall))
	h.mux.HandleFunc("POST /calls/{callID}/terminate", h.authenticate(h.terminateCall))
	h.mux.HandleFunc("POST /validate", h.authenticate(h.validate))
	h.mux.HandleFunc("GET /health", h.health)

	return h.mux
}

type tenantHandlerFunc func(http.ResponseWriter, *http.Request, *tenant.Tenant)

// Simple Auth middleware
func (h *Handler) authenticate(next tenantHandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		t := h.tenants.Authenticate(r.Header.Get(HeaderPhoneNumber), r.Header.Get(HeaderApiKey))
		if t == nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		next(w, r, t)
	}
}

// Health that return data and publicly accessed. HHH smell.
func (h *Handler) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":       "ok",
		"active_calls": h.registry.Count(),
	})
}

// validate returns true if the tenant credentials are valid.
func (h *Handler) validate(w http.ResponseWriter, _ *http.Request, _ *tenant.Tenant) {
	writeJSON(w, http.StatusOK, map[string]bool{"valid": true})
}

func (h *Handler) initiateCall(w http.ResponseWriter, r *http.Request, t *tenant.Tenant) {
	var req InitiateCallRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.To == "" || req.From == "" || req.SDPOffer == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "to, from, and sdp_offer are required"})
		return
	}

	if req.From != t.PhoneNumber {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	callID := uuid.New().String()
	log.Info().Str("call_id", callID).Str("to", req.To).Str("from", req.From).Msg("initiating outbound call")

	// we are generting sdpAnswer and send it directly to simplify the cycel
	// but in real-life we would send it later via webhooks if session answered.
	session, sdpAnswer, err := h.cs.PrepareOutboundSession(callID, req.To, req.From, req.SDPOffer)
	if err != nil {
		log.Error().Err(err).Str("call_id", callID).Msg("failed to prepare outbound call")
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to prepare outbound call"})
		return
	}

	bridge.StartMediaBridge(session.Ctx, session, nil)
	writeJSON(w, http.StatusOK, map[string]string{"call_id": callID, "sdp_answer": sdpAnswer})

	go h.cs.HandleOutboundAsync(session, req.From, sdpAnswer)
}

// Accept incoming call
func (h *Handler) acceptCall(w http.ResponseWriter, r *http.Request, t *tenant.Tenant) {
	callID := r.PathValue("callID")
	session := h.registry.Get(callID)
	if session == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "call not found"})
		return
	}

	if !h.ownsSession(t, session) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	var req AcceptCallRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.SDPAnswer == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "sdp_answer is required"})
		return
	}

	if err := h.sip.AcceptInbound(session, req.SDPAnswer); err != nil {
		log.Error().Err(err).Str("call_id", callID).Msg("failed to accept inbound call")
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	bridge.StartMediaBridge(session.Ctx, session, session.Recorder)
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

// Reject incoming call
func (h *Handler) rejectCall(w http.ResponseWriter, r *http.Request, t *tenant.Tenant) {
	callID := r.PathValue("callID")
	session := h.registry.Get(callID)
	if session == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "call not found"})
		return
	}

	if !h.ownsSession(t, session) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	if err := h.sip.RejectInbound(session); err != nil {
		log.Error().Err(err).Str("call_id", callID).Msg("failed to reject call")
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

// Terminate call (send BYE) for answered incoming or outbound calls
func (h *Handler) terminateCall(w http.ResponseWriter, r *http.Request, t *tenant.Tenant) {
	callID := r.PathValue("callID")
	session := h.registry.Get(callID)
	if session == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "call not found"})
		return
	}

	if !h.ownsSession(t, session) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	h.sip.SendBye(session)
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

// ownsSession checks that the authenticated tenant owns the call session.
func (h *Handler) ownsSession(t *tenant.Tenant, session *call.Session) bool {
	if session.IsInbound() {
		return session.To == t.PhoneNumber
	}
	return session.From == t.PhoneNumber
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
