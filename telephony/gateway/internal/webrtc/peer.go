// Package webrtc manages WebRTC peer connections for the SIP gateway.
package webrtc

import (
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/pion/webrtc/v4"
	"github.com/rs/zerolog/log"
)

const (
	// iceGatherTimeout is the maximum time to wait for ICE candidate gathering.
	iceGatherTimeout = 10 * time.Second

	// trackID identifies the audio track in WebRTC sessions.
	trackID = "audio"

	// trackStreamID identifies the media stream in WebRTC sessions.
	trackStreamID = "sip-gateway"

	// iceTCPMuxBufferSize is the buffer size for ICE TCP multiplexing.
	iceTCPMuxBufferSize = 8

	// pcmuClockRate is the sample rate for PCMU audio.
	pcmuClockRate = 8000

	// pcmuChannels is the number of channels for PCMU audio.
	pcmuChannels = 1

	// pcmuPayloadType is the RTP payload type for PCMU.
	pcmuPayloadType = 0
)

var (
	apiOnce     sync.Once
	apiInstance *webrtc.API
	apiInitErr  error
)

// InitAPI initializes the shared WebRTC API singleton with the given
// public IP and ICE TCP port. Must be called before creating peer connections.
func InitAPI(publicIP string, iceTCPPort int, ICENATIP string) error {
	apiOnce.Do(func() {
		apiInstance, apiInitErr = buildAPI(publicIP, iceTCPPort, ICENATIP)
	})
	return apiInitErr
}

func getAPI() (*webrtc.API, error) {
	if apiInstance == nil {
		return nil, errors.New("webrtc API not initialized; call InitAPI first")
	}
	return apiInstance, nil
}

func buildAPI(publicIP string, iceTCPPort int, ICENATIP string) (*webrtc.API, error) {
	m := &webrtc.MediaEngine{}
	if err := m.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:  webrtc.MimeTypePCMU,
			ClockRate: pcmuClockRate,
			Channels:  pcmuChannels,
		},
		PayloadType: pcmuPayloadType,
	}, webrtc.RTPCodecTypeAudio); err != nil {
		return nil, fmt.Errorf("register PCMU codec: %w", err)
	}

	settingEngine := webrtc.SettingEngine{}
	settingEngine.SetNAT1To1IPs([]string{ICENATIP}, webrtc.ICECandidateTypeHost)

	tcpListener, err := net.ListenTCP("tcp", &net.TCPAddr{
		IP:   net.ParseIP(publicIP),
		Port: iceTCPPort,
	})
	if err != nil {
		return nil, fmt.Errorf("listen ICE TCP on %s:%d: %w", publicIP, iceTCPPort, err)
	}

	log.Info().Str("addr", tcpListener.Addr().String()).Msg("WebRTC ICE TCP listener started")

	tcpMux := webrtc.NewICETCPMux(nil, tcpListener, iceTCPMuxBufferSize)
	settingEngine.SetICETCPMux(tcpMux)
	settingEngine.SetNetworkTypes([]webrtc.NetworkType{
		webrtc.NetworkTypeTCP4,
	})

	return webrtc.NewAPI(
		webrtc.WithSettingEngine(settingEngine),
		webrtc.WithMediaEngine(m),
	), nil
}

func newPeerConnection(stunServer string) (*webrtc.PeerConnection, error) {
	api, err := getAPI()
	if err != nil {
		return nil, err
	}

	pc, err := api.NewPeerConnection(webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{URLs: []string{stunServer}},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("create peer connection: %w", err)
	}
	return pc, nil
}

func addLocalTrack(pc *webrtc.PeerConnection) (*webrtc.TrackLocalStaticRTP, error) {
	track, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{
			MimeType:  webrtc.MimeTypePCMU,
			ClockRate: pcmuClockRate,
			Channels:  pcmuChannels,
		},
		trackID,
		trackStreamID,
	)
	if err != nil {
		return nil, fmt.Errorf("create local track: %w", err)
	}

	if _, err = pc.AddTrack(track); err != nil {
		return nil, fmt.Errorf("add local track: %w", err)
	}
	return track, nil
}

func waitForGatheringComplete(pc *webrtc.PeerConnection) error {
	gatherComplete := webrtc.GatheringCompletePromise(pc)
	select {
	case <-gatherComplete:
		return nil
	case <-time.After(iceGatherTimeout):
		return fmt.Errorf("ICE gathering timed out after %s", iceGatherTimeout)
	}
}

// NewPeerAsOfferer creates a WebRTC peer connection that generates an SDP offer.
// Returns the peer connection, local audio track, and SDP offer string.
func NewPeerAsOfferer(stunServer string) (*webrtc.PeerConnection, *webrtc.TrackLocalStaticRTP, string, error) {
	pc, err := newPeerConnection(stunServer)
	if err != nil {
		return nil, nil, "", err
	}

	track, err := addLocalTrack(pc)
	if err != nil {
		pc.Close()
		return nil, nil, "", err
	}

	offer, err := pc.CreateOffer(nil)
	if err != nil {
		pc.Close()
		return nil, nil, "", fmt.Errorf("create offer: %w", err)
	}

	if err := pc.SetLocalDescription(offer); err != nil {
		pc.Close()
		return nil, nil, "", fmt.Errorf("set local description: %w", err)
	}

	if err := waitForGatheringComplete(pc); err != nil {
		pc.Close()
		return nil, nil, "", err
	}

	return pc, track, pc.LocalDescription().SDP, nil
}

// NewPeerAsAnswerer creates a WebRTC peer connection that answers a browser's SDP offer.
// Returns the peer connection, local audio track, and SDP answer string.
func NewPeerAsAnswerer(stunServer, browserOffer string) (*webrtc.PeerConnection, *webrtc.TrackLocalStaticRTP, string, error) {
	pc, err := newPeerConnection(stunServer)
	if err != nil {
		return nil, nil, "", err
	}

	track, err := addLocalTrack(pc)
	if err != nil {
		pc.Close()
		return nil, nil, "", err
	}

	if err = pc.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  browserOffer,
	}); err != nil {
		pc.Close()
		return nil, nil, "", fmt.Errorf("set remote description: %w", err)
	}

	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		pc.Close()
		return nil, nil, "", fmt.Errorf("create answer: %w", err)
	}

	if err := pc.SetLocalDescription(answer); err != nil {
		pc.Close()
		return nil, nil, "", fmt.Errorf("set local description: %w", err)
	}

	if err := waitForGatheringComplete(pc); err != nil {
		pc.Close()
		return nil, nil, "", err
	}

	return pc, track, pc.LocalDescription().SDP, nil
}
