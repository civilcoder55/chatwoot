package sip

import (
	"gateway/internal/call"
	"testing"
)

func TestExtractPhone(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"plain number", "1234567890", "1234567890"},
		{"with plus prefix", "+1234567890", "+1234567890"},
		{"with dashes", "123-456-7890", "1234567890"},
		{"with spaces", "123 456 7890", "1234567890"},
		{"with parens", "(123) 456-7890", "1234567890"},
		{"sip username no digits", "alice", "alice"},
		{"empty string", "", ""},
		{"mixed alpha and digits", "user+123", "+123"},
		{"sip URI user", "alice@example.com", "alice@example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractPhone(tt.input)
			if got != tt.expected {
				t.Errorf("extractPhone(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestParseSDPMedia(t *testing.T) {
	tests := []struct {
		name         string
		sdp          string
		expectedIP   string
		expectedPort int
	}{
		{
			"standard SDP",
			"v=0\r\no=- 123 1 IN IP4 10.0.0.1\r\ns=-\r\nc=IN IP4 10.0.0.1\r\nt=0 0\r\nm=audio 5004 RTP/AVP 0\r\n",
			"10.0.0.1",
			5004,
		},
		{
			"missing connection line",
			"v=0\r\nm=audio 5004 RTP/AVP 0\r\n",
			"",
			5004,
		},
		{
			"missing media line",
			"v=0\r\nc=IN IP4 10.0.0.1\r\n",
			"10.0.0.1",
			0,
		},
		{
			"empty SDP",
			"",
			"",
			0,
		},
		{
			"localhost address",
			"c=IN IP4 127.0.0.1\r\nm=audio 8000 RTP/AVP 0\r\n",
			"127.0.0.1",
			8000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip, port := parseSDPMedia(tt.sdp)
			if ip != tt.expectedIP || port != tt.expectedPort {
				t.Errorf("parseSDPMedia() = (%q, %d), want (%q, %d)", ip, port, tt.expectedIP, tt.expectedPort)
			}
		})
	}
}

func TestParsePort(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"5060", 5060},
		{"0", 0},
		{"65535", 65535},
		{"abc", 0},
		{"", 0},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := parsePort(tt.input); got != tt.expected {
				t.Errorf("parsePort(%q) = %d, want %d", tt.input, got, tt.expected)
			}
		})
	}
}

func TestLocalPhoneNumber(t *testing.T) {
	inbound := &call.Session{Direction: call.DirectionInbound, From: "+111", To: "+222"}
	if got := localPhoneNumber(inbound); got != "+222" {
		t.Errorf("localPhoneNumber(inbound) = %q, want +222", got)
	}

	outbound := &call.Session{Direction: call.DirectionOutbound, From: "+111", To: "+222"}
	if got := localPhoneNumber(outbound); got != "+111" {
		t.Errorf("localPhoneNumber(outbound) = %q, want +111", got)
	}
}

func TestGenerateSIPSDP(t *testing.T) {
	sdp := generateSIPSDP("192.168.1.1", 5004)

	ip, port := parseSDPMedia(sdp)
	if ip != "192.168.1.1" {
		t.Errorf("generated SDP connection IP = %q, want %q", ip, "192.168.1.1")
	}
	if port != 5004 {
		t.Errorf("generated SDP media port = %d, want %d", port, 5004)
	}
}
