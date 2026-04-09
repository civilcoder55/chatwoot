package call

import "testing"

func TestRegistryAddGetRemove(t *testing.T) {
	r := NewRegistry()

	session := &Session{CallID: "test-1"}
	r.Add(session)

	got := r.Get("test-1")
	if got == nil {
		t.Fatal("expected to find session, got nil")
	}
	if got.CallID != "test-1" {
		t.Errorf("CallID = %q, want %q", got.CallID, "test-1")
	}

	if r.Get("nonexistent") != nil {
		t.Error("expected nil for nonexistent call ID")
	}

	r.Remove("test-1")
	if r.Get("test-1") != nil {
		t.Error("expected nil after removal")
	}
}

func TestRegistryCount(t *testing.T) {
	r := NewRegistry()

	if r.Count() != 0 {
		t.Errorf("Count() = %d, want 0", r.Count())
	}

	r.Add(&Session{CallID: "a"})
	r.Add(&Session{CallID: "b"})

	if r.Count() != 2 {
		t.Errorf("Count() = %d, want 2", r.Count())
	}

	r.Remove("a")
	if r.Count() != 1 {
		t.Errorf("Count() = %d, want 1", r.Count())
	}
}

func TestRegistryRange(t *testing.T) {
	r := NewRegistry()
	r.Add(&Session{CallID: "x"})
	r.Add(&Session{CallID: "y"})

	seen := map[string]bool{}
	r.Range(func(s *Session) bool {
		seen[s.CallID] = true
		return true
	})

	if len(seen) != 2 {
		t.Errorf("Range visited %d sessions, want 2", len(seen))
	}
}

func TestRegistryRangeEarlyStop(t *testing.T) {
	r := NewRegistry()
	r.Add(&Session{CallID: "a"})
	r.Add(&Session{CallID: "b"})
	r.Add(&Session{CallID: "c"})

	count := 0
	r.Range(func(_ *Session) bool {
		count++
		return false // stop after first
	})

	if count != 1 {
		t.Errorf("Range with early stop visited %d sessions, want 1", count)
	}
}

func TestRegistryGetBySIPCallID(t *testing.T) {
	r := NewRegistry()
	s := &Session{CallID: "gw-1", SIPCallID: "sip-abc@host"}
	r.Add(s)

	got := r.GetBySIPCallID("sip-abc@host")
	if got == nil {
		t.Fatal("expected to find session by SIP Call-ID")
	}
	if got.CallID != "gw-1" {
		t.Errorf("CallID = %q, want gw-1", got.CallID)
	}

	if r.GetBySIPCallID("nonexistent") != nil {
		t.Error("expected nil for unknown SIP Call-ID")
	}
}

func TestRegistryIndexSIPCallID(t *testing.T) {
	r := NewRegistry()
	s := &Session{CallID: "gw-1"}
	r.Add(s)

	// SIP Call-ID not known at Add time
	if r.GetBySIPCallID("sip-later") != nil {
		t.Error("expected nil before IndexSIPCallID")
	}

	r.IndexSIPCallID("gw-1", "sip-later")
	got := r.GetBySIPCallID("sip-later")
	if got == nil || got.CallID != "gw-1" {
		t.Error("expected to find session after IndexSIPCallID")
	}
}

func TestRegistryRemoveCleansUpSIPIndex(t *testing.T) {
	r := NewRegistry()
	s := &Session{CallID: "gw-1", SIPCallID: "sip-1"}
	r.Add(s)

	r.Remove("gw-1")

	if r.GetBySIPCallID("sip-1") != nil {
		t.Error("expected SIP Call-ID index to be cleaned up after Remove")
	}
}
