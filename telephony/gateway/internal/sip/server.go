// Package sip implements the SIP signaling server that bridges telephony
// with WebRTC through the Chatwoot gateway.
package sip

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"gateway/internal/call"
	"gateway/internal/config"
	"gateway/internal/tenant"
	"gateway/internal/webhook"

	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
	"github.com/rs/zerolog/log"
)

const (
	// userAgentName is the SIP User-Agent header value.
	userAgentName = "sip-gateway"

	// clientBindAddress is the local address for the SIP client socket.
	clientBindAddress = "192.168.1.6:5085"
)

// Server handles SIP signaling for inbound and outbound calls.
type Server struct {
	cfg       *config.Config
	registry  *call.Registry
	webhook   *webhook.Client
	tenants   *tenant.Store
	server    *sipgo.Server
	ua        *sipgo.UserAgent
	dialogSrv *sipgo.DialogServerCache
	dialogCli *sipgo.DialogClientCache
}

// NewServer creates a SIP server with inbound/outbound call handling.
func NewServer(cfg *config.Config, registry *call.Registry, wh *webhook.Client, ts *tenant.Store) (*Server, error) {
	ua, err := sipgo.NewUA(sipgo.WithUserAgent(userAgentName))
	if err != nil {
		return nil, fmt.Errorf("create SIP user agent: %w", err)
	}

	srv, err := sipgo.NewServer(ua)
	if err != nil {
		return nil, fmt.Errorf("create SIP server: %w", err)
	}

	client, err := sipgo.NewClient(ua)
	if err != nil {
		return nil, fmt.Errorf("create SIP client: %w", err)
	}

	contact := gatewayContact(cfg)
	s := &Server{
		cfg:       cfg,
		registry:  registry,
		webhook:   wh,
		tenants:   ts,
		server:    srv,
		ua:        ua,
		dialogSrv: sipgo.NewDialogServerCache(client, contact),
		dialogCli: sipgo.NewDialogClientCache(client, contact),
	}

	srv.OnInvite(s.handleInvite)
	srv.OnAck(s.handleAck)
	srv.OnBye(s.handleBye)
	srv.OnCancel(s.handleCancel)

	return s, nil
}

// Listen starts the SIP server on the configured address.
func (s *Server) Listen(ctx context.Context) error {
	addr := s.cfg.SIPAddr()
	log.Info().Str("addr", addr).Msg("SIP server listening")
	return s.server.ListenAndServe(ctx, "udp", addr)
}

func (s *Server) EnableDebugMode() {
	sip.SIPDebug = true
	sip.TransactionFSMDebug = true
	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})
	logger := slog.New(handler)
	slog.SetDefault(logger)
}

func gatewayContact(cfg *config.Config) sip.ContactHeader {
	return sip.ContactHeader{
		Address: sip.Uri{
			Host: cfg.EffectiveIP(),
			Port: parsePort(cfg.SIPPort),
		},
	}
}
