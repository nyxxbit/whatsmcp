package rest

// DTOs keep the exact JSON keys of the legacy bridge, so existing API consumers
// server e o whisper-tool dependem deste contrato. Não renomear campos.

type sendRequest struct {
	Recipient string `json:"recipient"`
	Message   string `json:"message"`
	MediaPath string `json:"media_path,omitempty"`
}

type sendResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

type downloadRequest struct {
	MessageID string `json:"message_id"`
	ChatJID   string `json:"chat_jid"`
}

type downloadResponse struct {
	Success  bool   `json:"success"`
	Message  string `json:"message"`
	Filename string `json:"filename,omitempty"`
	Path     string `json:"path,omitempty"`
}
