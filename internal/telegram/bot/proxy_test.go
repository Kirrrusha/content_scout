package bot

import "testing"

func TestNewHTTPClientNoProxy(t *testing.T) {
	client, err := newHTTPClient("")
	if err != nil {
		t.Fatalf("newHTTPClient(\"\") returned error: %v", err)
	}
	if client.Transport != nil {
		t.Fatalf("expected default transport when no proxy configured, got %#v", client.Transport)
	}
}

func TestNewHTTPClientWithSocks5Proxy(t *testing.T) {
	client, err := newHTTPClient("socks5://user:pass@127.0.0.1:1080")
	if err != nil {
		t.Fatalf("newHTTPClient with socks5 url returned error: %v", err)
	}
	if client.Transport == nil {
		t.Fatal("expected a custom transport when proxy is configured")
	}
}

func TestNewHTTPClientInvalidURL(t *testing.T) {
	if _, err := newHTTPClient("://not-a-url"); err == nil {
		t.Fatal("expected error for invalid proxy url")
	}
}
