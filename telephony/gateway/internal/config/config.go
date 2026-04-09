// Package config provides configuration for the SIP gateway.
package config

import (
	"os"
	"strconv"

	"github.com/rs/zerolog/log"
)

// Environment variable names.
const (
	EnvSIPPort       = "GATEWAY_SIP_PORT"
	EnvHTTPPort      = "GATEWAY_HTTP_PORT"
	EnvSTUNServer    = "GATEWAY_STUN_SERVER"
	EnvRecordingDir  = "GATEWAY_RECORDING_DIR"
	EnvWebhookURL    = "WEBHOOK_URL"
	EnvWebhookSecret = "WEBHOOK_SECRET"
	EnvSIPServerHost = "SIP_SERVER_HOST"
	EnvSIPServerPort = "SIP_SERVER_PORT"
	EnvICETCPPort    = "GATEWAY_ICE_TCP_PORT"
	EnvICETCPAddr    = "GATEWAY_ICE_TCP_ADDR"
	EnvICENATIP      = "GATEWAY_ICE_NAT_IP"
	EnvTenantsFile   = "GATEWAY_TENANTS_FILE"
	EnvBindAddr      = "GATEWAY_BIND_ADDR"
)

// Default configuration values.
const (
	DefaultSIPPort       = "5080"
	DefaultHTTPPort      = "8080"
	DefaultSTUNServer    = "stun:stun.l.google.com:19302"
	DefaultRecordingDir  = "/recordings"
	DefaultWebhookURL    = "http://127.0.0.1:3000/webhooks/sip_gateway/events"
	DefaultWebhookSecret = "test"
	DefaultSIPServerHost = "127.0.0.1"
	DefaultSIPServerPort = "5060"
	DefaultICETCPPort    = 9565
	DefaultFallbackIP    = "0.0.0.0"
	DefaultICETCPAddr    = "0.0.0.0"
	DefaultICENATIP      = ""
	DefaultTenantsFile   = "tenants.yaml"
	DefaultBindAddr      = "0.0.0.0"
)

// Config holds all gateway configuration values.
type Config struct {
	SIPPort       string
	HTTPPort      string
	STUNServer    string
	RecordingDir  string
	WebhookURL    string
	WebhookSecret string
	SIPServerHost string
	SIPServerPort string
	PublicIP      string
	ICETCPPort    int
	ICETCPAddr    string
	ICENATIP      string
	TenantsFile   string
	BindAddr      string
}

// Load reads configuration from environment variables, falling back to defaults.
func Load() *Config {
	cfg := &Config{
		SIPPort:       envOr(EnvSIPPort, DefaultSIPPort),
		HTTPPort:      envOr(EnvHTTPPort, DefaultHTTPPort),
		STUNServer:    envOr(EnvSTUNServer, DefaultSTUNServer),
		RecordingDir:  envOr(EnvRecordingDir, DefaultRecordingDir),
		WebhookURL:    envOr(EnvWebhookURL, DefaultWebhookURL),
		WebhookSecret: envOr(EnvWebhookSecret, DefaultWebhookSecret),
		SIPServerHost: envOr(EnvSIPServerHost, DefaultSIPServerHost),
		SIPServerPort: envOr(EnvSIPServerPort, DefaultSIPServerPort),
		ICETCPPort:    envOrInt(EnvICETCPPort, DefaultICETCPPort),
		ICETCPAddr:    envOr(EnvICETCPAddr, DefaultICETCPAddr),
		ICENATIP:      envOr(EnvICENATIP, DefaultICENATIP),
		TenantsFile:   envOr(EnvTenantsFile, DefaultTenantsFile),
		BindAddr:      envOr(EnvBindAddr, DefaultBindAddr),
	}

	cfg.log()
	return cfg
}

// EffectiveIP returns the PublicIP or a localhost fallback if empty.
func (c *Config) EffectiveIP() string {
	if c.PublicIP != "" {
		return c.PublicIP
	}
	return DefaultFallbackIP
}

// HTTPAddr returns the full HTTP listen address (bind_addr:port).
func (c *Config) HTTPAddr() string {
	return c.BindAddr + ":" + c.HTTPPort
}

// SIPAddr returns the full SIP listen address (bind_addr:port).
func (c *Config) SIPAddr() string {
	return c.BindAddr + ":" + c.SIPPort
}

func (c *Config) log() {
	log.Info().
		Str("sip_addr", c.SIPAddr()).
		Str("http_addr", c.HTTPAddr()).
		Str("stun_server", c.STUNServer).
		Str("recording_dir", c.RecordingDir).
		Str("webhook_url", c.WebhookURL).
		Str("sip_server", c.SIPServerHost+":"+c.SIPServerPort).
		Int("ice_tcp_port", c.ICETCPPort).
		Str("ice_tcp_addr", c.ICETCPAddr).
		Msg("configuration loaded")
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envOrInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		log.Warn().Str("key", key).Str("value", v).Msg("invalid integer env var, using default")
		return fallback
	}
	return n
}
