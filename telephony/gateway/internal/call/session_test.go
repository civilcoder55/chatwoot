package call

import "testing"

func TestStatusIsTerminal(t *testing.T) {
	terminal := []Status{StatusEnded, StatusRejected, StatusMissed, StatusFailed}
	for _, s := range terminal {
		if !s.IsTerminal() {
			t.Errorf("Status(%q).IsTerminal() = false, want true", s)
		}
	}

	nonTerminal := []Status{StatusRinging, StatusAccepted}
	for _, s := range nonTerminal {
		if s.IsTerminal() {
			t.Errorf("Status(%q).IsTerminal() = true, want false", s)
		}
	}
}

func TestSessionSetGetStatus(t *testing.T) {
	s := &Session{Status: StatusRinging}

	if got := s.GetStatus(); got != StatusRinging {
		t.Errorf("GetStatus() = %q, want %q", got, StatusRinging)
	}

	s.SetStatus(StatusAccepted)
	if got := s.GetStatus(); got != StatusAccepted {
		t.Errorf("GetStatus() = %q, want %q", got, StatusAccepted)
	}
}

func TestSessionIsTerminal(t *testing.T) {
	s := &Session{Status: StatusRinging}
	if s.IsTerminal() {
		t.Error("expected non-terminal for ringing session")
	}

	s.SetStatus(StatusEnded)
	if !s.IsTerminal() {
		t.Error("expected terminal for ended session")
	}
}

func TestSessionIsInboundOutbound(t *testing.T) {
	inbound := &Session{Direction: DirectionInbound}
	if !inbound.IsInbound() {
		t.Error("expected IsInbound() = true")
	}
	if inbound.IsOutbound() {
		t.Error("expected IsOutbound() = false for inbound session")
	}

	outbound := &Session{Direction: DirectionOutbound}
	if !outbound.IsOutbound() {
		t.Error("expected IsOutbound() = true")
	}
	if outbound.IsInbound() {
		t.Error("expected IsInbound() = false for outbound session")
	}
}

func TestSessionLocalPhoneNumber(t *testing.T) {
	inbound := &Session{Direction: DirectionInbound, From: "+111", To: "+222"}
	if got := inbound.LocalPhoneNumber(); got != "+222" {
		t.Errorf("inbound LocalPhoneNumber() = %q, want +222", got)
	}

	outbound := &Session{Direction: DirectionOutbound, From: "+111", To: "+222"}
	if got := outbound.LocalPhoneNumber(); got != "+111" {
		t.Errorf("outbound LocalPhoneNumber() = %q, want +111", got)
	}
}
