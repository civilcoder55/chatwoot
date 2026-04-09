package sip

import (
	"fmt"
	"net"
	"time"

	"gateway/internal/call"
	"gateway/internal/config"
	"gateway/internal/recording"

	"github.com/rs/zerolog/log"
)

// setupAcceptedCall performs the common post-answer setup shared by inbound and
// outbound call flows: opens the RTP socket, transitions the session to accepted,
// and starts the call recorder.
func setupAcceptedCall(session *call.Session, cfg *config.Config) (sipSDP string, err error) {
	conn, err := net.ListenPacket("udp4", ":0")
	if err != nil {
		return "", fmt.Errorf("open RTP socket: %w", err)
	}
	session.RTPConn = conn

	localPort := conn.LocalAddr().(*net.UDPAddr).Port
	sipSDP = generateSIPSDP(cfg.EffectiveIP(), localPort)

	session.SetStatus(call.StatusAccepted)
	session.StartedAt = time.Now()

	recorder := recording.NewRecorder(cfg.RecordingDir, session.CallID)
	if err := recorder.Start(); err != nil {
		log.Warn().Err(err).Str("call_id", session.CallID).Msg("failed to start recording")
	} else {
		session.Recorder = recorder
	}

	return sipSDP, nil
}
