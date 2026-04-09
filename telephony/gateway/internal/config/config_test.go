package config

import (
	"os"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	// Unset any env vars that would override defaults.
	for _, key := range []string{EnvSIPPort, EnvHTTPPort, EnvICETCPPort} {
		os.Unsetenv(key)
	}

	cfg := Load()

	if cfg.SIPPort != DefaultSIPPort {
		t.Errorf("SIPPort = %q, want %q", cfg.SIPPort, DefaultSIPPort)
	}
	if cfg.HTTPPort != DefaultHTTPPort {
		t.Errorf("HTTPPort = %q, want %q", cfg.HTTPPort, DefaultHTTPPort)
	}
	if cfg.ICETCPPort != DefaultICETCPPort {
		t.Errorf("ICETCPPort = %d, want %d", cfg.ICETCPPort, DefaultICETCPPort)
	}
}

func TestLoadFromEnv(t *testing.T) {
	os.Setenv(EnvSIPPort, "6060")
	defer os.Unsetenv(EnvSIPPort)

	cfg := Load()
	if cfg.SIPPort != "6060" {
		t.Errorf("SIPPort = %q, want %q", cfg.SIPPort, "6060")
	}
}

func TestEffectiveIP(t *testing.T) {
	cfg := &Config{PublicIP: "10.0.0.1"}
	if got := cfg.EffectiveIP(); got != "10.0.0.1" {
		t.Errorf("EffectiveIP() = %q, want %q", got, "10.0.0.1")
	}

	cfg.PublicIP = ""
	if got := cfg.EffectiveIP(); got != DefaultFallbackIP {
		t.Errorf("EffectiveIP() = %q, want %q", got, DefaultFallbackIP)
	}
}

func TestHTTPAddr(t *testing.T) {
	cfg := &Config{PublicIP: "10.0.0.1", HTTPPort: "9090"}
	if got := cfg.HTTPAddr(); got != "10.0.0.1:9090" {
		t.Errorf("HTTPAddr() = %q, want %q", got, "10.0.0.1:9090")
	}
}

func TestSIPAddr(t *testing.T) {
	cfg := &Config{PublicIP: "10.0.0.1", SIPPort: "5080"}
	if got := cfg.SIPAddr(); got != "10.0.0.1:5080" {
		t.Errorf("SIPAddr() = %q, want %q", got, "10.0.0.1:5080")
	}
}
