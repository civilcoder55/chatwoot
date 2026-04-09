package sip

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"gateway/internal/bridge"
	"gateway/internal/call"
	"gateway/internal/webhook"

	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

const (
	// inviteTimeout is how long to wait for the remote party to answer.
	inviteTimeout = 30 * time.Second

	// sipStatusSessionProgress is the SIP 183 Session Progress response.
	sipStatusSessionProgress = 183
)

// SendInvite initiates an outbound SIP call.
func (s *Server) SendInvite(ctx context.Context, session *call.Session) error {
	gatewayIP := s.cfg.EffectiveIP()

	conn, err := net.ListenPacket("udp4", ":0")
	if err != nil {
		return fmt.Errorf("open RTP socket: %w", err)
	}
	session.RTPConn = conn

	localPort := conn.LocalAddr().(*net.UDPAddr).Port
	sipSDP := generateSIPSDP(gatewayIP, localPort)
	target := fmt.Sprintf("sip:%s@%s:%s", session.To, s.cfg.SIPServerHost, s.cfg.SIPServerPort)

	req := sip.NewRequest(sip.INVITE, sip.Uri{
		User: session.To,
		Host: s.cfg.SIPServerHost,
		Port: parsePort(s.cfg.SIPServerPort),
	})
	req.SetBody([]byte(sipSDP))
	req.AppendHeader(sip.NewHeader("Content-Type", "application/sdp"))
	req.AppendHeader(sip.NewHeader("From", fmt.Sprintf("<sip:%s@%s>;tag=%s", session.From, gatewayIP, uuid.New().String()[:8])))
	req.AppendHeader(sip.NewHeader("To", fmt.Sprintf("<%s>", target)))

	log.Info().
		Str("call_id", session.CallID).
		Str("target", target).
		Str("from", session.From).
		Msg("sending outbound SIP INVITE")

	dialog, err := s.dialogCli.WriteInvite(ctx, req)
	if err != nil {
		return fmt.Errorf("send INVITE: %w", err)
	}

	session.OutboundDlg = dialog
	sipCallID := callIDValue(dialog.InviteRequest)
	session.SIPCallID = sipCallID
	s.registry.IndexSIPCallID(session.CallID, sipCallID)

	inviteCtx, inviteCancel := context.WithTimeout(ctx, inviteTimeout)
	go func() {
		defer inviteCancel()
		s.handleOutboundAnswer(inviteCtx, session, dialog)
	}()

	return nil
}

func (s *Server) handleOutboundAnswer(ctx context.Context, session *call.Session, dialog *sipgo.DialogClientSession) {
	ringingNotified := false

	err := dialog.WaitAnswer(ctx, sipgo.AnswerOptions{
		OnResponse: func(res *sip.Response) error {
			code := res.StatusCode
			log.Info().
				Int("sip_status", int(code)).
				Str("call_id", session.CallID).
				Msg("outbound INVITE response")

			if (code == sip.StatusRinging || code == sipStatusSessionProgress) && !ringingNotified {
				ringingNotified = true
				if err := s.webhook.Send(webhook.EventRinging, map[string]any{
					"call_id":      session.CallID,
					"phone_number": localPhoneNumber(session),
				}); err != nil {
					log.Error().Err(err).Str("call_id", session.CallID).Msg("failed to send call.ringing webhook")
				}
			}
			return nil
		},
	})
	if err != nil {
		s.handleOutboundError(session, err)
		return
	}

	if err := dialog.Ack(context.Background()); err != nil {
		log.Error().Err(err).Str("call_id", session.CallID).Msg("failed to send ACK for outbound 200 OK")
		s.terminateSession(session, "sip-ack-failed")
		return
	}

	if remoteIP, remotePort := parseSDPMedia(string(dialog.InviteResponse.Body())); remoteIP != "" && remotePort > 0 {
		session.RTPAddr = &net.UDPAddr{IP: net.ParseIP(remoteIP), Port: remotePort}
	}

	if _, err := setupAcceptedCall(session, s.cfg); err != nil {
		log.Error().Err(err).Str("call_id", session.CallID).Msg("failed to set up accepted outbound call")
		s.terminateSession(session, "setup-failed")
		return
	}

	log.Info().Str("call_id", session.CallID).Msg("outbound call answered")

	if err := s.webhook.Send(webhook.EventAnswered, map[string]any{
		"call_id":      session.CallID,
		"phone_number": localPhoneNumber(session),
	}); err != nil {
		log.Error().Err(err).Str("call_id", session.CallID).Msg("failed to send call.answered webhook")
	}

	bridge.StartMediaBridge(session.Ctx, session)
}

func (s *Server) handleOutboundError(session *call.Session, err error) {
	if session.IsTerminal() {
		return
	}

	reason := "sip-transaction-failed"
	extra := map[string]any{}

	var dialogErr *sipgo.ErrDialogResponse
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		reason = "no-answer"
	case errors.Is(err, context.Canceled):
		reason = hangupReason(InitiatorAgent, false)
	case errors.As(err, &dialogErr):
		code := int(dialogErr.Res.StatusCode)
		reason = sipFailureReason(code)
		extra["sip_code"] = code
	default:
		log.Error().Err(err).Str("call_id", session.CallID).Msg("outbound INVITE failed")
	}

	s.terminateSessionWithData(session, reason, extra)
}
