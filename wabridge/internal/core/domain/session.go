package domain

// SessionState é o estado da sessão com o WhatsApp.
type SessionState string

const (
	// SessionConnectedState: logado e online.
	SessionConnectedState SessionState = "connected"
	// SessionOfflineState: sessão válida porém desconectada (basta reconectar).
	SessionOfflineState SessionState = "offline"
	// SessionLoggedOutState: sem sessão; precisa parear via QR.
	SessionLoggedOutState SessionState = "logged_out"
)

// SessionStatus é um Value Object imutável com o estado atual da sessão e a
// conta (número) autenticada, quando houver.
type SessionStatus struct {
	state   SessionState
	account string
}

// NewSessionStatus cria o status (sem validação: todos os estados são válidos).
func NewSessionStatus(state SessionState, account string) SessionStatus {
	return SessionStatus{state: state, account: account}
}

// State devolve o estado da sessão.
func (s SessionStatus) State() SessionState { return s.state }

// Account devolve a conta autenticada (vazio se deslogado).
func (s SessionStatus) Account() string { return s.account }

// IsConnected indica se está logado e online.
func (s SessionStatus) IsConnected() bool { return s.state == SessionConnectedState }
