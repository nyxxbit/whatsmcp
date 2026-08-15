package rest

import (
	"encoding/json"
	"net/http"

	"github.com/nyxxbit/wabridge/internal/core/domain"
)

// handleSend recria POST /api/send: texto puro (message) ou mídia (media_path,
// com message virando legenda). Resposta {success, message}, contrato do legado.
func (s *Server) handleSend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req sendRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request format", http.StatusBadRequest)
		return
	}
	if req.Recipient == "" {
		http.Error(w, "Recipient is required", http.StatusBadRequest)
		return
	}
	if req.Message == "" && req.MediaPath == "" {
		http.Error(w, "Message or media path is required", http.StatusBadRequest)
		return
	}

	to, err := domain.ParseRecipient(req.Recipient)
	if err != nil {
		writeSend(w, false, "Error parsing recipient: "+err.Error())
		return
	}

	ctx := r.Context()
	if req.MediaPath != "" {
		err = s.sender.SendMedia(ctx, to, req.MediaPath, req.Message)
	} else {
		err = s.sender.SendText(ctx, to, req.Message)
	}
	if err != nil {
		s.log.Warn("falha ao enviar", "para", req.Recipient, "err", err)
		writeSend(w, false, err.Error())
		return
	}
	writeSend(w, true, "Message sent to "+req.Recipient)
}

func writeSend(w http.ResponseWriter, success bool, message string) {
	w.Header().Set("Content-Type", "application/json")
	if !success {
		w.WriteHeader(http.StatusInternalServerError)
	}
	_ = json.NewEncoder(w).Encode(sendResponse{Success: success, Message: message})
}

// handleDownload recria POST /api/download: baixa (ou usa cache) a mídia de uma
// mensagem e devolve {success, message, filename, path}.
func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req downloadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request format", http.StatusBadRequest)
		return
	}
	if req.MessageID == "" || req.ChatJID == "" {
		http.Error(w, "Message ID and Chat JID are required", http.StatusBadRequest)
		return
	}

	res, err := s.downloader.Download(r.Context(), req.MessageID, req.ChatJID)
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		s.log.Warn("falha no download", "msg", req.MessageID, "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(downloadResponse{
			Success: false,
			Message: "Failed to download media: " + err.Error(),
		})
		return
	}
	_ = json.NewEncoder(w).Encode(downloadResponse{
		Success:  true,
		Message:  "Successfully downloaded " + string(res.Kind()) + " media",
		Filename: res.Filename(),
		Path:     res.Path(),
	})
}

// handleSyncLabels recria /api/sync-labels: dispara o fullSync de etiquetas.
func (s *Server) handleSyncLabels(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if err := s.labelSyncer.SyncLabels(r.Context()); err != nil {
		_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "error": err.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "message": "label sync triggered (regular fullSync)"})
}
