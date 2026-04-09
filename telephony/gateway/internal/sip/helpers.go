package sip

import (
	"fmt"
	"math/rand"
	"regexp"
	"strconv"
	"strings"

	"gateway/internal/call"

	"github.com/emiago/sipgo/sip"
)

const (
	// sdpSessionName is the session name used in generated SDP.
	sdpSessionName = "sip-gateway"

	// sdpMaxSessionID is the upper bound for random SDP session IDs.
	sdpMaxSessionID = 999999999
)

var (
	sdpConnectionRe = regexp.MustCompile(`c=IN IP4 (\S+)`)
	sdpMediaRe      = regexp.MustCompile(`m=audio (\d+)`)
)

// generateSIPSDP produces a minimal SDP body for a PCMU audio session.
func generateSIPSDP(ip string, port int) string {
	sessionID := rand.Int63n(sdpMaxSessionID)
	return fmt.Sprintf(`v=0
o=- %d 1 IN IP4 %s
s=%s
c=IN IP4 %s
t=0 0
m=audio %d RTP/AVP 0
a=rtpmap:0 PCMU/8000
a=sendrecv
`, sessionID, ip, sdpSessionName, ip, port)
}

// parseSDPMedia extracts the connection IP and audio port from an SDP body.
func parseSDPMedia(sdp string) (string, int) {
	var ip string
	var port int

	if m := sdpConnectionRe.FindStringSubmatch(sdp); len(m) > 1 {
		ip = m[1]
	}
	if m := sdpMediaRe.FindStringSubmatch(sdp); len(m) > 1 {
		if p, err := strconv.Atoi(m[1]); err == nil {
			port = p
		}
	}
	return ip, port
}

// extractPhone strips non-numeric characters (except +) from a SIP user field.
func extractPhone(user string) string {
	cleaned := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' || r == '+' {
			return r
		}
		return -1
	}, user)
	if cleaned == "" {
		return user
	}
	return cleaned
}

// parsePort converts a string port to an int, returning 0 on failure.
func parsePort(s string) int {
	p, _ := strconv.Atoi(s)
	return p
}

// localPhoneNumber returns the phone number that belongs to the local SIP channel.
func localPhoneNumber(session *call.Session) string {
	return session.LocalPhoneNumber()
}

// callIDValue extracts the Call-ID header value from a SIP request.
func callIDValue(req *sip.Request) string {
	if req == nil || req.CallID() == nil {
		return ""
	}
	return req.CallID().Value()
}
