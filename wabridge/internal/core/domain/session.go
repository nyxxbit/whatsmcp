package domain

// SessionState is the state of the WhatsApp session.
type SessionState string

const (
	// SessionConnectedState: logged in and online.
	SessionConnectedState SessionState = "connected"
	// SessionOfflineState: valid session but disconnected (just needs reconnecting).
	SessionOfflineState SessionState = "offline"
	// SessionLoggedOutState: no session; needs pairing via QR.
	SessionLoggedOutState SessionState = "logged_out"
)

// SessionStatus is an immutable Value Object with the current session state and the
// authenticated account (number), when there is one.
type SessionStatus struct {
	state   SessionState
	account string
}

// NewSessionStatus creates the status (no validation: all states are valid).
func NewSessionStatus(state SessionState, account string) SessionStatus {
	return SessionStatus{state: state, account: account}
}

// State returns the session's state.
func (s SessionStatus) State() SessionState { return s.state }

// Account returns the authenticated account (empty if logged out).
func (s SessionStatus) Account() string { return s.account }

// IsConnected reports whether it is logged in and online.
func (s SessionStatus) IsConnected() bool { return s.state == SessionConnectedState }
