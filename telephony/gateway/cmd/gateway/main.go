// Package main is the entry point for the SIP gateway server.
package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"gateway/internal/api"
	"gateway/internal/call"
	"gateway/internal/config"
	sipserver "gateway/internal/sip"
	"gateway/internal/tenant"
	"gateway/internal/webhook"
	gw "gateway/internal/webrtc"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout})

	cfg := config.Load()

	if err := gw.InitAPI(cfg.ICETCPAddr, cfg.ICETCPPort, cfg.ICENATIP); err != nil {
		log.Fatal().Err(err).Msg("failed to initialize WebRTC API")
	}

	// In memory registery, [IIHT] i'd depend on persistent layer like redis
	// also it could help if we scaled our gateway HZ
	registry := call.NewRegistry()
	wh := webhook.NewClient(cfg.WebhookURL, cfg.WebhookSecret)

	tenants, err := tenant.LoadStore(cfg.TenantsFile)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load tenants")
	}

	sipSrv, err := sipserver.NewServer(cfg, registry, wh, tenants)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create SIP server")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		if err := sipSrv.Listen(ctx); err != nil {
			log.Fatal().Err(err).Msg("SIP server failed")
		}
	}()

	handler := api.NewHandler(cfg, registry, sipSrv, wh, tenants)
	httpAddr := cfg.HTTPAddr()
	log.Info().Str("addr", httpAddr).Msg("HTTP API server listening")

	srv := &http.Server{Addr: httpAddr, Handler: handler}

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		sig := <-sigCh
		log.Info().Str("signal", sig.String()).Msg("shutting down")
		cancel()
		srv.Close()
	}()

	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatal().Err(err).Msg("HTTP server failed")
	}

	log.Info().Msg("gateway stopped")
}
