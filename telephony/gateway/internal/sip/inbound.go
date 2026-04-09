package sip

import (
	"context"
	"fmt"
	"net"
	"time"

	"gateway/internal/call"
	"gateway/internal/recording"
	gw "gateway/internal/webrtc"

	"github.com/emiago/sipgo/sip"
	"github.com/google/uuid"
	"github.com/pion/webrtc/v4"
	"github.com/rs/zerolog/log"
)

// handleInvite processes an incoming SIP INVITE request.
func (s *Server) handleInvite(req *sip.Request, tx sip.ServerTransaction) {
	dlg, err := s.dialogSrv.ReadInvite(req, tx)
	if err != nil {
		log.Error().Err(err).Msg("failed to establish inbound dialog")
		_ = tx.Respond(sip.NewResponseFromRequest(req, sip.StatusBadRequest, "Bad Request", nil))
		return
	}

	if err := dlg.Respond(sip.StatusTrying, "Trying", nil); err != nil {
		log.Error().Err(err).Msg("failed to send 100 Trying")
		return
	}

	from := extractPhone(req.From().Address.User)
	to := extractPhone(req.To().Address.User)
	callID := uuid.New().String()

	log.Info().
		Str("call_id", callID).
		Str("from", from).
		Str("to", to).
		Str("sip_call_id", callIDValue(dlg.InviteRequest)).
		Msg("incoming SIP INVITE")

	pc, localTrack, sdpOffer, err := gw.NewPeerAsOfferer(s.cfg.STUNServer)
	if err != nil {
		log.Error().Err(err).Str("call_id", callID).Msg("failed to create WebRTC peer for inbound call")
		if respondErr := dlg.Respond(sip.StatusInternalServerError, "Server Error", nil); respondErr != nil {
			log.Error().Err(respondErr).Msg("failed to send 500 for inbound INVITE")
		}
		return
	}

	sessionCtx, cancel := context.WithCancel(context.Background())
	session := &call.Session{
		CallID:     callID,
		SIPCallID:  callIDValue(dlg.InviteRequest),
		Direction:  call.DirectionInbound,
		Status:     call.StatusRinging,
		From:       from,
		To:         to,
		Ctx:        sessionCtx,
		PC:         pc,
		LocalTrack: localTrack,
		CreatedAt:  time.Now(),
		Cancel:     cancel,
		InboundOps: make(chan call.InboundAction, 1),
		InboundDlg: dlg,
	}

	s.registry.Add(session)

	if remoteIP, remotePort := parseSDPMedia(string(req.Body())); remoteIP != "" && remotePort > 0 {
		session.RTPAddr = &net.UDPAddr{IP: net.ParseIP(remoteIP), Port: remotePort}
	}

	if err := dlg.Respond(sip.StatusRinging, "Ringing", nil); err != nil {
		log.Error().Err(err).Str("call_id", callID).Msg("failed to send 180 Ringing")
		s.terminateSession(session, "sip-setup-failed")
		return
	}

	if err := s.webhook.Send("call.incoming", map[string]any{
		"call_id":      callID,
		"from":         from,
		"to":           to,
		"sdp_offer":    sdpOffer,
		"phone_number": to,
	}); err != nil {
		log.Error().Err(err).Str("call_id", callID).Msg("failed to send call.incoming webhook")
	}

	// Block until an accept/reject action arrives or the dialog is cancelled.
	for {
		select {
		case action := <-session.InboundOps:
			var actionErr error
			switch action.Type {
			case call.InboundActionAccept:
				actionErr = s.answerInbound(session, action.SDPAnswer)
			case call.InboundActionReject:
				actionErr = s.rejectInboundInvite(session)
			default:
				actionErr = fmt.Errorf("unsupported inbound action %q", action.Type)
			}
			action.Result <- actionErr
			close(action.Result)
			return
		case <-dlg.Context().Done():
			return
		}
	}
}

// handleAck processes an incoming SIP ACK request.
func (s *Server) handleAck(req *sip.Request, tx sip.ServerTransaction) {
	if err := s.dialogSrv.ReadAck(req, tx); err != nil {
		log.Debug().Err(err).Msg("failed to match ACK to inbound dialog")
		return
	}

	if session := s.findSessionBySIPCallID(req); session != nil {
		log.Debug().Str("call_id", session.CallID).Msg("received ACK")
	}
}

// AcceptInbound accepts an inbound call with the given browser SDP answer.
func (s *Server) AcceptInbound(session *call.Session, browserSDP string) error {
	return s.dispatchInboundAction(session, call.InboundAction{
		Type:      call.InboundActionAccept,
		SDPAnswer: browserSDP,
		Result:    make(chan error, 1),
	})
}

// RejectInbound rejects an inbound call.
func (s *Server) RejectInbound(session *call.Session) error {
	return s.dispatchInboundAction(session, call.InboundAction{
		Type:   call.InboundActionReject,
		Result: make(chan error, 1),
	})
}

func (s *Server) dispatchInboundAction(session *call.Session, action call.InboundAction) error {
	if session == nil || session.InboundDlg == nil {
		return fmt.Errorf("no inbound dialog for session")
	}
	if session.InboundOps == nil {
		return fmt.Errorf("inbound operation channel is not initialized")
	}

	select {
	case session.InboundOps <- action:
	case <-session.Ctx.Done():
		return context.Cause(session.Ctx)
	}

	return <-action.Result
}

func (s *Server) answerInbound(session *call.Session, browserSDP string) error {
	if err := session.PC.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.SDPTypeAnswer,
		SDP:  browserSDP,
	}); err != nil {
		return fmt.Errorf("set remote description: %w", err)
	}

	conn, err := net.ListenPacket("udp4", ":0")
	if err != nil {
		return fmt.Errorf("open RTP socket: %w", err)
	}
	session.RTPConn = conn

	localPort := conn.LocalAddr().(*net.UDPAddr).Port
	sipSDP := generateSIPSDP(s.cfg.EffectiveIP(), localPort)

	session.SetStatus(call.StatusAccepted)
	session.StartedAt = time.Now()

	recorder := recording.NewRecorder(s.cfg.RecordingDir, session.CallID)
	if err := recorder.Start(); err != nil {
		log.Warn().Err(err).Str("call_id", session.CallID).Msg("failed to start recording")
	} else {
		session.Recorder = recorder
	}

	if err := session.InboundDlg.RespondSDP([]byte(sipSDP)); err != nil {
		if session.IsTerminal() {
			return nil
		}
		log.Error().Err(err).Str("call_id", session.CallID).Msg("failed to answer inbound INVITE")
		s.terminateSession(session, "sip-accept-failed")
		return err
	}

	log.Info().Str("call_id", session.CallID).Msg("inbound call accepted")

	go func(callID, phoneNumber string) {
		if err := s.webhook.Send("call.accepted", map[string]any{
			"call_id":      callID,
			"phone_number": phoneNumber,
		}); err != nil {
			log.Error().Err(err).Str("call_id", callID).Msg("failed to send call.accepted webhook")
		}
	}(session.CallID, localPhoneNumber(session))

	return nil
}

func (s *Server) rejectInboundInvite(session *call.Session) error {
	session.SetStatus(call.StatusRejected)

	if err := session.InboundDlg.Respond(sip.StatusBusyHere, "Busy Here", nil); err != nil {
		if session.IsTerminal() {
			return nil
		}
		log.Error().Err(err).Str("call_id", session.CallID).Msg("failed to reject inbound INVITE")
		return err
	}

	log.Info().Str("call_id", session.CallID).Msg("inbound call rejected")
	session.Close()
	s.registry.Remove(session.CallID)
	return nil
}
