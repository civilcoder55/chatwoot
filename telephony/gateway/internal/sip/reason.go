package sip

import (
	"fmt"

	"gateway/internal/call"
)

// sipFailureReason maps SIP failure status codes to human-readable reasons.
func sipFailureReason(code int) string {
	switch code {
	case 486:
		return "busy"
	case 480, 408:
		return "no-answer"
	case 487:
		return "remote-canceled"
	case 603:
		return "rejected"
	case 404:
		return "not-found"
	case 503:
		return "service-unavailable"
	default:
		return fmt.Sprintf("sip-%d", code)
	}
}

// hangupReason returns a descriptive reason string based on who initiated the
// hangup and whether the call was already connected.
func hangupReason(initiator string, connected bool) string {
	switch initiator {
	case "agent":
		if connected {
			return "agent-hangup"
		}
		return "agent-canceled"
	case "remote":
		if connected {
			return "remote-hangup"
		}
		return "remote-canceled"
	default:
		return "failed"
	}
}

// statusForReason maps a termination reason to the appropriate call status.
func statusForReason(reason string) call.Status {
	switch reason {
	case "agent-rejected", "agent-canceled", "busy", "rejected":
		return call.StatusRejected
	case "remote-canceled", "no-answer":
		return call.StatusMissed
	case "agent-hangup", "remote-hangup":
		return call.StatusEnded
	default:
		return call.StatusFailed
	}
}
