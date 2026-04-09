package sip

import (
	"testing"

	"gateway/internal/call"
)

func TestHangupReason(t *testing.T) {
	tests := []struct {
		name      string
		initiator string
		connected bool
		expected  string
	}{
		{"agent cancels before answer", "agent", false, "agent-canceled"},
		{"agent ends connected call", "agent", true, "agent-hangup"},
		{"remote cancels before answer", "remote", false, "remote-canceled"},
		{"remote ends connected call", "remote", true, "remote-hangup"},
		{"unknown initiator connected", "unknown", true, "failed"},
		{"unknown initiator not connected", "unknown", false, "failed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hangupReason(tt.initiator, tt.connected); got != tt.expected {
				t.Errorf("hangupReason(%q, %t) = %q, want %q", tt.initiator, tt.connected, got, tt.expected)
			}
		})
	}
}

func TestStatusForReason(t *testing.T) {
	tests := []struct {
		reason   string
		expected call.Status
	}{
		{"agent-canceled", call.StatusRejected},
		{"agent-rejected", call.StatusRejected},
		{"busy", call.StatusRejected},
		{"rejected", call.StatusRejected},
		{"remote-canceled", call.StatusMissed},
		{"no-answer", call.StatusMissed},
		{"agent-hangup", call.StatusEnded},
		{"remote-hangup", call.StatusEnded},
		{"sip-500", call.StatusFailed},
		{"webrtc-setup-failed", call.StatusFailed},
		{"unknown-reason", call.StatusFailed},
	}

	for _, tt := range tests {
		t.Run(tt.reason, func(t *testing.T) {
			if got := statusForReason(tt.reason); got != tt.expected {
				t.Errorf("statusForReason(%q) = %q, want %q", tt.reason, got, tt.expected)
			}
		})
	}
}

func TestSipFailureReason(t *testing.T) {
	tests := []struct {
		name     string
		code     int
		expected string
	}{
		{"busy", 486, "busy"},
		{"temporarily unavailable", 480, "no-answer"},
		{"request timeout", 408, "no-answer"},
		{"request terminated", 487, "remote-canceled"},
		{"decline", 603, "rejected"},
		{"not found", 404, "not-found"},
		{"service unavailable", 503, "service-unavailable"},
		{"server error fallback", 500, "sip-500"},
		{"unknown code", 999, "sip-999"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sipFailureReason(tt.code); got != tt.expected {
				t.Errorf("sipFailureReason(%d) = %q, want %q", tt.code, got, tt.expected)
			}
		})
	}
}
