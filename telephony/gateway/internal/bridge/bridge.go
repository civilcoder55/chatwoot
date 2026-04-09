// Package bridge provides RTP bridging between SIP and WebRTC.
package bridge

import (
	"context"
	"net"
	"time"

	"gateway/internal/call"
	"gateway/internal/recording"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
	"github.com/rs/zerolog/log"
)

const (
	// rtpBufferSize is the maximum size of a single RTP packet.
	rtpBufferSize = 1500

	// readDeadline controls how frequently the SIP→WebRTC loop checks for
	// context cancellation. A shorter deadline is more responsive to shutdown
	// but uses slightly more CPU.
	readDeadline = 200 * time.Millisecond

	// waitForSIPConnInterval is how often the bridge re-checks for the SIP RTP
	// socket before the outbound INVITE has been fully set up.
	waitForSIPConnInterval = 50 * time.Millisecond
)

// StartMediaBridge launches bidirectional RTP forwarding between SIP and WebRTC.
func StartMediaBridge(ctx context.Context, session *call.Session, _ *recording.Recorder) {
	log.Info().Str("call_id", session.CallID).Msg("starting RTP bridge")
	session.StartSIPToWebRTC(func() {
		go sipToWebRTC(ctx, session)
	})
	session.StartWebRTCToSIP(func() {
		go webrtcToSIP(ctx, session)
	})
}

// sipToWebRTC reads RTP from the SIP connection and writes to the WebRTC track.
func sipToWebRTC(ctx context.Context, session *call.Session) {
	buf := make([]byte, rtpBufferSize)

	for {
		if ctx.Err() != nil {
			return
		}

		if session.RTPConn == nil {
			time.Sleep(waitForSIPConnInterval)
			continue
		}

		// Use a read deadline instead of spawning a goroutine per read.
		// This allows periodic context cancellation checks without goroutine overhead.
		if err := session.RTPConn.SetReadDeadline(time.Now().Add(readDeadline)); err != nil {
			log.Warn().Err(err).Str("call_id", session.CallID).Msg("failed to set SIP read deadline")
			return
		}

		n, _, err := session.RTPConn.ReadFrom(buf)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			if isTimeout(err) {
				continue
			}
			log.Warn().Err(err).Str("call_id", session.CallID).Msg("SIP RTP read error")
			continue
		}

		pkt := &rtp.Packet{}
		if err := pkt.Unmarshal(buf[:n]); err != nil {
			continue
		}

		if _, err := session.LocalTrack.Write(buf[:n]); err != nil {
			log.Warn().Err(err).Str("call_id", session.CallID).Msg("write to WebRTC track failed")
		}

		if session.Recorder != nil {
			session.Recorder.WritePacket(recording.StreamRemote, pkt.Timestamp, time.Now(), pkt.Payload)
		}
	}
}

// webrtcToSIP reads RTP from the WebRTC remote track and sends to the SIP endpoint.
func webrtcToSIP(ctx context.Context, session *call.Session) {
	session.PC.OnTrack(func(remoteTrack *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		log.Debug().Str("call_id", session.CallID).Msg("WebRTC remote track received")
		buf := make([]byte, rtpBufferSize)

		for {
			if ctx.Err() != nil {
				return
			}

			n, _, err := remoteTrack.Read(buf)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				log.Warn().Err(err).Str("call_id", session.CallID).Msg("WebRTC track read error")
				return
			}

			if session.RTPConn != nil && session.RTPAddr != nil {
				if _, err := session.RTPConn.WriteTo(buf[:n], session.RTPAddr); err != nil {
					log.Warn().Err(err).Str("call_id", session.CallID).Msg("write to SIP RTP failed")
				}
			}

			if session.Recorder != nil {
				pkt := &rtp.Packet{}
				if err := pkt.Unmarshal(buf[:n]); err == nil {
					session.Recorder.WritePacket(recording.StreamLocal, pkt.Timestamp, time.Now(), pkt.Payload)
				}
			}
		}
	})
}

func isTimeout(err error) bool {
	netErr, ok := err.(net.Error)
	return ok && netErr.Timeout()
}
