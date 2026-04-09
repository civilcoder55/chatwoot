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
	"gateway/internal/webhook"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

type Handler struct {
	cfg      *config.Config
	registry *call.Registry
	sip      *sipserver.Server
	webhook  *webhook.Client
	mux      *http.ServeMux
	cs       *services.CallService
}

// NewHandler creates a new HTTP handler with all routes registered.
func NewHandler(cfg *config.Config, registry *call.Registry, sip *sipserver.Server, wh *webhook.Client) http.Handler {
	cs := services.NewService(registry, sip, wh, cfg)

	h := &Handler{
		cfg:      cfg,
		registry: registry,
		sip:      sip,
		webhook:  wh,
		mux:      http.NewServeMux(),
		cs:       cs,
	}

	h.mux.HandleFunc("POST /calls/initiate", h.authenticate(h.initiateCall))
	h.mux.HandleFunc("POST /calls/{callID}/accept", h.authenticate(h.acceptCall))
	h.mux.HandleFunc("POST /calls/{callID}/reject", h.authenticate(h.rejectCall))
	h.mux.HandleFunc("POST /calls/{callID}/terminate", h.authenticate(h.terminateCall))
	h.mux.HandleFunc("GET /health", h.health)

	return h.mux
}

// Simple Auth middleware
func (h *Handler) authenticate(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(webhook.HeaderGatewaySecret) != h.cfg.WebhookSecret {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		next(w, r)
	}
}

// Health that return data and publicly accessed. HHH smell.
func (h *Handler) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":       "ok",
		"active_calls": h.registry.Count(),
	})
}

func (h *Handler) initiateCall(w http.ResponseWriter, r *http.Request) {
	var req InitiateCallRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.To == "" || req.From == "" || req.SDPOffer == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "to, from, and sdp_offer are required"})
		return
	}

	callID := uuid.New().String()
	log.Info().Str("call_id", callID).Str("to", req.To).Str("from", req.From).Msg("initiating outbound call")

	// we are generting sdpAnswer and send it directly to simplify the cycel
	// but in real-life we would send it later via webhooks if session answered.
	session, sdpAnswer, err := h.cs.PrepareOutboundSession(callID, req.To, req.From, req.SDPOffer)
	bridge.StartMediaBridge(session.Ctx, session, nil)
	if err != nil {
		log.Error().Err(err).Str("call_id", callID).Msg("failed to prepare outbound call")
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to prepare outbound call"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"call_id": callID, "sdp_answer": sdpAnswer})

	go h.cs.HandleOutboundAsync(session, req.From, sdpAnswer)
}

// Accept incoming call
func (h *Handler) acceptCall(w http.ResponseWriter, r *http.Request) {
	callID := r.PathValue("callID")
	session := h.registry.Get(callID)
	if session == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "call not found"})
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
func (h *Handler) rejectCall(w http.ResponseWriter, r *http.Request) {
	callID := r.PathValue("callID")
	session := h.registry.Get(callID)
	if session == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "call not found"})
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
func (h *Handler) terminateCall(w http.ResponseWriter, r *http.Request) {
	callID := r.PathValue("callID")
	session := h.registry.Get(callID)
	if session == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "call not found"})
		return
	}

	h.sip.SendBye(session)
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
