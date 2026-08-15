package messaging_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/nyxxbit/wabridge/internal/core/domain"
	"github.com/nyxxbit/wabridge/internal/features/messaging"
	"github.com/nyxxbit/wabridge/internal/platform/logging"
)

type fakeMediaRepo struct {
	media domain.Media
	err   error
}

func (r fakeMediaRepo) Save(context.Context, domain.Message) error      { return nil }
func (r fakeMediaRepo) SaveBatch(context.Context, []domain.Message) error { return nil }
func (r fakeMediaRepo) FindMedia(context.Context, string, string) (domain.Media, error) {
	return r.media, r.err
}

type countingFetcher struct {
	data  []byte
	calls int
}

func (f *countingFetcher) Fetch(context.Context, domain.Media) ([]byte, error) {
	f.calls++
	return f.data, nil
}

func completeMedia(t *testing.T) domain.Media {
	t.Helper()
	m, err := domain.NewMedia(domain.MediaSpec{
		Kind: domain.MediaDocument, Filename: "report.pdf", URL: "https://x.net/abc.enc",
		MediaKey: []byte("k"), FileSHA256: []byte("s"), FileEncSHA256: []byte("e"), FileLength: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestDownloader_baixaEUsaCache(t *testing.T) {
	dir := t.TempDir()
	fetcher := &countingFetcher{data: []byte("pdf")}
	d := messaging.NewDownloader(fakeMediaRepo{media: completeMedia(t)}, fetcher, dir, logging.Noop{})

	res, err := d.Download(context.Background(), "ID", "5511@s.whatsapp.net")
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if res.Filename() != "report.pdf" {
		t.Fatalf("filename = %q", res.Filename())
	}
	if _, err := os.Stat(filepath.Join(dir, "5511@s.whatsapp.net", "report.pdf")); err != nil {
		t.Fatalf("arquivo não salvo: %v", err)
	}

	// Segunda chamada: cache hit, fetcher NÃO deve ser chamado de novo.
	if _, err := d.Download(context.Background(), "ID", "5511@s.whatsapp.net"); err != nil {
		t.Fatalf("Download (cache): %v", err)
	}
	if fetcher.calls != 1 {
		t.Fatalf("fetcher chamado %d vezes, esperava 1 (cache)", fetcher.calls)
	}
}

func TestDownloader_propagaMidiaInexistente(t *testing.T) {
	d := messaging.NewDownloader(fakeMediaRepo{err: domain.ErrMediaNotFound}, &countingFetcher{}, t.TempDir(), logging.Noop{})
	if _, err := d.Download(context.Background(), "ID", "chat"); !errors.Is(err, domain.ErrMediaNotFound) {
		t.Fatalf("esperava ErrMediaNotFound, veio %v", err)
	}
}

func TestDownloader_recusaMetadadosIncompletos(t *testing.T) {
	incomplete, _ := domain.NewMedia(domain.MediaSpec{Kind: domain.MediaImage, Filename: "x.jpg"}) // sem url/keys
	d := messaging.NewDownloader(fakeMediaRepo{media: incomplete}, &countingFetcher{}, t.TempDir(), logging.Noop{})
	if _, err := d.Download(context.Background(), "ID", "chat"); err == nil {
		t.Fatal("esperava erro de metadados incompletos")
	}
}
