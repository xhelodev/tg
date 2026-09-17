package telegram

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSendMedia_Multipart(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pic.jpg")
	if err := os.WriteFile(path, []byte("fakejpegdata"), 0o644); err != nil {
		t.Fatal(err)
	}

	var gotCT string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCT = r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		if !strings.Contains(r.URL.Path, "sendPhoto") {
			t.Errorf("path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
			"result": map[string]any{
				"message_id": 7,
				"date":       1,
				"chat":       map[string]any{"id": 1, "type": "private"},
			},
		})
	}))
	defer srv.Close()

	c, _ := New(validToken, WithBaseURL(srv.URL), WithHTTPClient(srv.Client()), WithLimiter(nil))
	msg, err := c.SendMedia(context.Background(), SendMediaRequest{
		ChatID: "1", Kind: MediaPhoto, FilePath: path, Caption: "cap",
	})
	if err != nil {
		t.Fatal(err)
	}
	if msg.MessageID != 7 {
		t.Fatal(msg.MessageID)
	}
	if !strings.HasPrefix(gotCT, "multipart/form-data") {
		t.Fatalf("CT=%s", gotCT)
	}
	if !strings.Contains(string(gotBody), "fakejpegdata") {
		t.Fatal("body sin archivo")
	}
	if !strings.Contains(string(gotBody), `name="photo"`) {
		t.Fatal("falta campo photo")
	}
}

func TestSendMedia_FileID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"photo":"AgAC"`) {
			t.Errorf("body=%s", body)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
			"result": map[string]any{
				"message_id": 1, "date": 1,
				"chat": map[string]any{"id": 1, "type": "private"},
			},
		})
	}))
	defer srv.Close()
	c, _ := New(validToken, WithBaseURL(srv.URL), WithHTTPClient(srv.Client()), WithLimiter(nil))
	_, err := c.SendMedia(context.Background(), SendMediaRequest{
		ChatID: "1", Kind: MediaPhoto, FileID: "AgAC",
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSendDocument_URLRestriction(t *testing.T) {
	c, _ := New(validToken, WithLimiter(nil))
	_, err := c.SendMedia(context.Background(), SendMediaRequest{
		ChatID: "1", Kind: MediaDocument, URL: "https://example.com/file.txt",
	})
	if err == nil || !strings.Contains(err.Error(), ".PDF") {
		t.Fatalf("esperado rechazo PDF/ZIP, got %v", err)
	}
}

func TestSendMedia_SizeLimit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "big.bin")
	// Crear archivo > límite de foto multipart sin gastar mucha RAM: truncar
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(MaxPhotoMultipart + 1); err != nil {
		t.Fatal(err)
	}
	f.Close()

	c, _ := New(validToken, WithLimiter(nil))
	_, err = c.SendMedia(context.Background(), SendMediaRequest{
		ChatID: "1", Kind: MediaPhoto, FilePath: path,
	})
	if err == nil || !strings.Contains(err.Error(), "supera el límite") {
		t.Fatalf("esperado límite, got %v", err)
	}
}
