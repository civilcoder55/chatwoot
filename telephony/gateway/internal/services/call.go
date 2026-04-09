package services

import (
	"context"
	"fmt"
	"gateway/internal/call"
	"gateway/internal/config"
	sipserver "gateway/internal/sip"
	"gateway/internal/webhook"
	"time"

	gw "gateway/internal/webrtc"

	"github.com/rs/zerolog/log"
)

type CallService struct {
	registry *call.Registry
	sip      *sipserver.Server
	webhook  *webhook.Client
	cfg      *config.Config
}

func NewService(registry *call.Registry, sip *sipserver.Server, webhook *webhook.Client, cfg *config.Config) *CallService {
	return &CallService{
		registry: registry,
		sip:      sip,
		webhook:  webhook,
		cfg:      cfg,
	}
}

func (cs *CallService) PrepareOutboundSession(callID, to, from, browserOffer string) (*call.Session, string, error) {
	pc, localTrack, sdpAnswer, err := gw.NewPeerAsAnswerer(cs.cfg.STUNServer, browserOffer)
	if err != nil {
		return nil, "", fmt.Errorf("create WebRTC peer for outbound call: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	session := &call.Session{
		CallID:     callID,
		Direction:  call.DirectionOutbound,
		Status:     call.StatusRinging,
		From:       from,
		To:         to,
		Ctx:        ctx,
		PC:         pc,
		LocalTrack: localTrack,
		CreatedAt:  time.Now(),
		Cancel:     cancel,
	}

	cs.registry.Add(session)

	return session, sdpAnswer, nil
}

func (cs *CallService) HandleOutboundAsync(session *call.Session, from, sdpAnswer string) {
	if err := cs.webhook.Send("call.sdp_answer", map[string]any{
		"call_id":      session.CallID,
		"sdp_answer":   sdpAnswer,
		"phone_number": from,
	}); err != nil {
		log.Warn().Err(err).Str("call_id", session.CallID).Msg("failed to send sdp_answer webhook")
	}

	if err := cs.sip.SendInvite(session.Ctx, session); err != nil {
		log.Error().Err(err).Str("call_id", session.CallID).Msg("failed to send SIP INVITE")
		cs.webhook.Send("call.ended", map[string]any{"call_id": session.CallID, "reason": "sip-invite-failed", "phone_number": from})

		if session.Cancel != nil {
			session.Cancel()
		}

		if session.PC != nil {
			session.PC.Close()
		}

		cs.registry.Remove(session.CallID)
		return
	}
}
