// Package call manages call sessions and their lifecycle.
package call

import (
	"context"
	"net"
	"sync"
	"time"

	"gateway/internal/recording"

	"github.com/emiago/sipgo"
	"github.com/pion/webrtc/v4"
)

type Session struct {
	mu              sync.Mutex
	sipToWebRTCOnce sync.Once
	webrtcToSIPOnce sync.Once
	CallID          string
	SIPCallID       string
	Direction       Direction
	Status          Status
	From            string
	To              string
	Ctx             context.Context
	PC              *webrtc.PeerConnection
	RTPConn         net.PacketConn // for receiving RTP Media
	RTPAddr         *net.UDPAddr   // to send RTP Media
	LocalTrack      *webrtc.TrackLocalStaticRTP
	Recorder        *recording.Recorder
	StartedAt       time.Time
	CreatedAt       time.Time
	Cancel          context.CancelFunc
	InboundOps      chan InboundAction
	InboundDlg      *sipgo.DialogServerSession
	OutboundDlg     *sipgo.DialogClientSession
}

// ensures the SIP->WebRTC bridge is only started once. (singleton)
func (s *Session) StartSIPToWebRTC(start func()) {
	s.sipToWebRTCOnce.Do(start)
}

func (s *Session) StartWebRTCToSIP(start func()) {
	s.webrtcToSIPOnce.Do(start)
}

func (s *Session) SetStatus(status Status) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Status = status
}

func (s *Session) GetStatus() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Status
}

func (s *Session) IsTerminal() bool {
	return s.GetStatus().IsTerminal()
}

func (s *Session) Close() {
	if s.Cancel != nil {
		s.Cancel()
	}
	if s.PC != nil {
		_ = s.PC.Close()
	}
	if s.RTPConn != nil {
		_ = s.RTPConn.Close()
	}
	if s.InboundDlg != nil {
		_ = s.InboundDlg.Close()
	}
	if s.OutboundDlg != nil {
		_ = s.OutboundDlg.Close()
	}
}

func (s *Session) IsInbound() bool {
	return s.Direction == DirectionInbound
}

func (s *Session) IsOutbound() bool {
	return s.Direction == DirectionOutbound
}

// LocalPhoneNumber returns the phone number belonging to the local tenant.
// For inbound calls that is the "to" number; for outbound calls the "from" number.
func (s *Session) LocalPhoneNumber() string {
	if s.Direction == DirectionInbound {
		return s.To
	}
	return s.From
}
