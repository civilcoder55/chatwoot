package sip

import (
	"context"
	"os"
	"time"

	"gateway/internal/call"
	"gateway/internal/webhook"

	"github.com/emiago/sipgo/sip"
	"github.com/rs/zerolog/log"
)

const (
	// byeTimeout is the maximum time to wait for a BYE response.
	byeTimeout = 5 * time.Second

	// sipStatusCallDoesNotExist is sent when a BYE references an unknown call.
	sipStatusCallDoesNotExist = 481

	// sipStatusRequestTerminated is sent after a CANCEL is processed.
	sipStatusRequestTerminated = 487
)

// handleBye processes an incoming SIP BYE request.
func (s *Server) handleBye(req *sip.Request, tx sip.ServerTransaction) {
	session := s.findSessionBySIPCallID(req)

	if err := s.dialogSrv.ReadBye(req, tx); err == nil {
		if session != nil {
			log.Info().Str("call_id", session.CallID).Msg("inbound BYE received, ending call")
			s.terminateSession(session, hangupReason(InitiatorRemote, true))
		}
		return
	}

	if err := s.dialogCli.ReadBye(req, tx); err == nil {
		if session != nil {
			log.Info().Str("call_id", session.CallID).Msg("outbound BYE received, ending call")
			s.terminateSession(session, hangupReason(InitiatorRemote, true))
		}
		return
	}

	log.Warn().Str("sip_call_id", callIDValue(req)).Msg("BYE received for unknown session")
	if err := tx.Respond(sip.NewResponseFromRequest(req, sipStatusCallDoesNotExist, "Call/Transaction Does Not Exist", nil)); err != nil {
		log.Error().Err(err).Msg("failed to send 481 for unknown BYE")
	}
}

// SendBye terminates a call by sending a SIP BYE or rejecting a ringing call.
func (s *Server) SendBye(session *call.Session) {
	if session == nil || session.IsTerminal() {
		return
	}

	status := session.GetStatus()

	// Handle ringing calls: reject or cancel instead of BYE.
	switch {
	case session.Direction == call.DirectionInbound && status == call.StatusRinging:
		if err := s.RejectInbound(session); err != nil {
			log.Error().Err(err).Str("call_id", session.CallID).Msg("failed to reject ringing inbound call")
		}
		return
	case session.Direction == call.DirectionOutbound && status == call.StatusRinging:
		if session.Cancel != nil {
			session.Cancel()
		}
		s.terminateSession(session, hangupReason(InitiatorAgent, false))
		return
	}

	// For connected calls, send a proper BYE.
	ctx, cancel := context.WithTimeout(context.Background(), byeTimeout)
	defer cancel()

	switch session.Direction {
	case call.DirectionInbound:
		if session.InboundDlg != nil {
			if err := session.InboundDlg.Bye(ctx); err != nil {
				log.Error().Err(err).Str("call_id", session.CallID).Msg("failed to send inbound BYE")
			}
		}
	case call.DirectionOutbound:
		if session.OutboundDlg != nil {
			if err := session.OutboundDlg.Bye(ctx); err != nil {
				log.Error().Err(err).Str("call_id", session.CallID).Msg("failed to send outbound BYE")
			}
		}
	}

	s.terminateSession(session, hangupReason(InitiatorAgent, true))
}

func (s *Server) terminateSession(session *call.Session, reason string) {
	s.terminateSessionWithData(session, reason, nil)
}

func (s *Server) terminateSessionWithData(session *call.Session, reason string, extra map[string]any) {
	if session == nil || session.IsTerminal() {
		return
	}

	var durationSeconds int
	if !session.StartedAt.IsZero() {
		durationSeconds = int(time.Since(session.StartedAt).Seconds())
	}

	session.SetStatus(statusForReason(reason))

	var recordingPath string
	if session.Recorder != nil {
		recordingPath = session.Recorder.Stop()
	}

	session.Close()
	s.registry.Remove(session.CallID)

	log.Info().
		Str("call_id", session.CallID).
		Str("reason", reason).
		Int("duration_seconds", durationSeconds).
		Str("recording", recordingPath).
		Msg("call terminated")

	data := map[string]any{
		"call_id":          session.CallID,
		"duration_seconds": durationSeconds,
		"reason":           reason,
		"phone_number":     localPhoneNumber(session),
	}
	for key, value := range extra {
		data[key] = value
	}

	if err := s.webhook.Send(webhook.EventEnded, data); err != nil {
		log.Error().Err(err).Str("call_id", session.CallID).Msg("failed to send call.ended webhook")
	}

	if recordingPath == "" {
		return
	}

	if err := s.webhook.UploadRecording(session.CallID, recordingPath); err != nil {
		log.Error().Err(err).Str("call_id", session.CallID).Str("recording", recordingPath).Msg("failed to upload recording")
		return
	}

	if err := os.Remove(recordingPath); err != nil {
		log.Warn().Err(err).Str("call_id", session.CallID).Str("recording", recordingPath).Msg("failed to delete uploaded recording")
	}
}

func (s *Server) findSessionBySIPCallID(req *sip.Request) *call.Session {
	sipCallID := callIDValue(req)
	if sipCallID == "" {
		return nil
	}
	return s.registry.GetBySIPCallID(sipCallID)
}
