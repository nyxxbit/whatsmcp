package rest_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nyxxbit/wabridge/internal/core/domain"
	"github.com/nyxxbit/wabridge/internal/delivery/rest"
	"github.com/nyxxbit/wabridge/internal/platform/logging"
)

type fakeSender struct {
	textTo, textBody                 string
	mediaTo, mediaPath, mediaCaption string
	err                              error
}

func (f *fakeSender) SendText(_ context.Context, to domain.JID, body string) error {
	f.textTo, f.textBody = to.String(), body
	return f.err
}
func (f *fakeSender) SendMedia(_ context.Context, to domain.JID, path, caption string) error {
	f.mediaTo, f.mediaPath, f.mediaCaption = to.String(), path, caption
	return f.err
}

type fakeDownloader struct {
	res domain.DownloadResult
	err error
}

func (f fakeDownloader) Download(context.Context, string, string) (domain.DownloadResult, error) {
	return f.res, f.err
}

type fakeSyncer struct{ err error }

func (f fakeSyncer) SyncLabels(context.Context) error { return f.err }

func newServer(t *testing.T, sender *fakeSender, dl fakeDownloader) http.Handler {
	t.Helper()
	logPath := filepath.Join(t.TempDir(), "wabridge.log")
	if err := os.WriteFile(logPath, []byte("log line\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := rest.NewServer(rest.Config{
		Sender: sender, Downloader: dl, LabelSyncer: fakeSyncer{},
		LogPath: logPath, Log: logging.Noop{},
	})
	if err != nil {
		t.Fatal(err)
	}
	return s.Handler()
}

func TestSend_textoOk(t *testing.T) {
	sender := &fakeSender{}
	srv := httptest.NewServer(newServer(t, sender, fakeDownloader{}))
	defer srv.Close()

	resp := postJSON(t, srv.URL+"/api/send", `{"recipient":"5511999990003","message":"good morning"}`)
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if out["success"] != true || out["message"] != "Message sent to 5511999990003" {
		t.Fatalf("unexpected response: %v", out)
	}
	if sender.textTo != "5511999990003@s.whatsapp.net" || sender.textBody != "good morning" {
		t.Fatalf("SendText received to=%q body=%q", sender.textTo, sender.textBody)
	}
}

func TestSend_midiaUsaCaption(t *testing.T) {
	sender := &fakeSender{}
	srv := httptest.NewServer(newServer(t, sender, fakeDownloader{}))
	defer srv.Close()

	resp := postJSON(t, srv.URL+"/api/send", `{"recipient":"5511999990003","message":"here is the report","media_path":"C:/x/report.pdf"}`)
	defer resp.Body.Close()

	if sender.mediaPath != "C:/x/report.pdf" || sender.mediaCaption != "here is the report" {
		t.Fatalf("SendMedia received path=%q caption=%q", sender.mediaPath, sender.mediaCaption)
	}
}

func TestSend_semRecipiente400(t *testing.T) {
	srv := httptest.NewServer(newServer(t, &fakeSender{}, fakeDownloader{}))
	defer srv.Close()
	resp := postJSON(t, srv.URL+"/api/send", `{"message":"hi"}`)
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("status = %d, expected 400", resp.StatusCode)
	}
}

func TestSend_erroDoSenderVira500(t *testing.T) {
	srv := httptest.NewServer(newServer(t, &fakeSender{err: errors.New("not connected")}, fakeDownloader{}))
	defer srv.Close()
	resp := postJSON(t, srv.URL+"/api/send", `{"recipient":"5511","message":"hi"}`)
	defer resp.Body.Close()
	if resp.StatusCode != 500 {
		t.Fatalf("status = %d, expected 500", resp.StatusCode)
	}
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if out["success"] != false {
		t.Fatalf("expected success:false, got %v", out)
	}
}

func TestDownload_ok(t *testing.T) {
	dl := fakeDownloader{res: domain.NewDownloadResult(domain.MediaAudio, "audio.ogg", "/abs/audio.ogg")}
	srv := httptest.NewServer(newServer(t, &fakeSender{}, dl))
	defer srv.Close()

	resp := postJSON(t, srv.URL+"/api/download", `{"message_id":"ABC","chat_jid":"5511@s.whatsapp.net"}`)
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if out["success"] != true || out["path"] != "/abs/audio.ogg" || out["filename"] != "audio.ogg" {
		t.Fatalf("unexpected response: %v", out)
	}
}

func TestDownload_camposFaltando400(t *testing.T) {
	srv := httptest.NewServer(newServer(t, &fakeSender{}, fakeDownloader{}))
	defer srv.Close()
	resp := postJSON(t, srv.URL+"/api/download", `{"message_id":"ABC"}`)
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("status = %d, expected 400", resp.StatusCode)
	}
}

func TestRoot_404_healthCheckContract(t *testing.T) {
	srv := httptest.NewServer(newServer(t, &fakeSender{}, fakeDownloader{}))
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("GET / = %d, health checks expect 404", resp.StatusCode)
	}
}

func TestLogs_serveTail(t *testing.T) {
	srv := httptest.NewServer(newServer(t, &fakeSender{}, fakeDownloader{}))
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/logs")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body := make([]byte, 64)
	n, _ := resp.Body.Read(body)
	if resp.StatusCode != 200 || !strings.Contains(string(body[:n]), "log line") {
		t.Fatalf("/logs status=%d body=%q", resp.StatusCode, string(body[:n]))
	}
}

func postJSON(t *testing.T, url, body string) *http.Response {
	t.Helper()
	resp, err := http.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	return resp
}
