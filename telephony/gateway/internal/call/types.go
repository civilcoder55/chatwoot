package call

type Direction string

const (
	DirectionInbound  Direction = "inbound"
	DirectionOutbound Direction = "outbound"
)

type Status string

const (
	StatusRinging  Status = "ringing"
	StatusAccepted Status = "accepted"
	StatusEnded    Status = "ended"
	StatusRejected Status = "rejected"
	StatusMissed   Status = "missed"
	StatusFailed   Status = "failed"
)

// IsTerminal returns true if the status represents a final state.
func (s Status) IsTerminal() bool {
	switch s {
	case StatusEnded, StatusRejected, StatusMissed, StatusFailed:
		return true
	}
	return false
}

type InboundActionType string

const (
	InboundActionAccept InboundActionType = "accept"
	InboundActionReject InboundActionType = "reject"
)

type InboundAction struct {
	Type      InboundActionType
	SDPAnswer string
	Result    chan error
}
