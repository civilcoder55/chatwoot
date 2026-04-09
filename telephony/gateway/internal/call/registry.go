package call

import "sync"

// Registry provides thread-safe storage for active call sessions.
type Registry struct {
	calls       sync.Map
	bySIPCallID sync.Map
}

// NewRegistry creates an empty call registry.
func NewRegistry() *Registry {
	return &Registry{}
}

// Add stores a session in the registry, keyed by its CallID.
// If the session already has a SIPCallID, it is also indexed.
func (r *Registry) Add(session *Session) {
	r.calls.Store(session.CallID, session)
	if session.SIPCallID != "" {
		r.bySIPCallID.Store(session.SIPCallID, session)
	}
}

// Get retrieves a session by CallID, returning nil if not found.
func (r *Registry) Get(callID string) *Session {
	v, ok := r.calls.Load(callID)
	if !ok {
		return nil
	}
	return v.(*Session)
}

// GetBySIPCallID retrieves a session by its SIP Call-ID header value.
func (r *Registry) GetBySIPCallID(sipCallID string) *Session {
	v, ok := r.bySIPCallID.Load(sipCallID)
	if !ok {
		return nil
	}
	return v.(*Session)
}

// IndexSIPCallID adds or updates the SIP Call-ID index for an existing session.
// Use this when the SIP Call-ID becomes known after the session was added.
func (r *Registry) IndexSIPCallID(callID, sipCallID string) {
	if s := r.Get(callID); s != nil {
		s.SIPCallID = sipCallID
		r.bySIPCallID.Store(sipCallID, s)
	}
}

// Remove deletes a session from the registry and its SIP Call-ID index.
func (r *Registry) Remove(callID string) {
	if s := r.Get(callID); s != nil && s.SIPCallID != "" {
		r.bySIPCallID.Delete(s.SIPCallID)
	}
	r.calls.Delete(callID)
}

// Count returns the number of active sessions.
func (r *Registry) Count() int {
	count := 0
	r.calls.Range(func(_, _ any) bool {
		count++
		return true
	})
	return count
}

// Range iterates over all sessions. Return false from fn to stop iteration.
func (r *Registry) Range(fn func(*Session) bool) {
	r.calls.Range(func(_, v any) bool {
		return fn(v.(*Session))
	})
}
