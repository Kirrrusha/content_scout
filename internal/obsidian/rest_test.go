package obsidian

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRESTClientWritesVaultNote(t *testing.T) {
	var auth string
	var path string
	var body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		path = r.URL.EscapedPath()
		raw, _ := io.ReadAll(r.Body)
		body = string(raw)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewRESTClient(server.URL, "key", false)
	err := client.WriteNote(context.Background(), "Articles/go/Go Guide.md", []byte("# Guide"))
	if err != nil {
		t.Fatalf("WriteNote() error = %v", err)
	}
	if auth != "Bearer key" {
		t.Fatalf("auth = %q", auth)
	}
	if path != "/vault/Articles/go/Go%20Guide.md" {
		t.Fatalf("path = %q", path)
	}
	if body != "# Guide" {
		t.Fatalf("body = %q", body)
	}
}

func TestEscapeVaultPathPreservesFolders(t *testing.T) {
	got := escapeVaultPath(`Articles\go/Go Guide.md`)
	if got != "Articles/go/Go%20Guide.md" {
		t.Fatalf("escapeVaultPath() = %q", got)
	}
}

func TestRESTClientReadNoteNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	_, err := NewRESTClient(server.URL, "key", false).ReadNote(context.Background(), "missing.md")
	if !errors.Is(err, ErrNoteNotFound) {
		t.Fatalf("ReadNote() error = %v, want ErrNoteNotFound", err)
	}
}
