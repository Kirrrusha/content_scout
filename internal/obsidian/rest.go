package obsidian

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var ErrNoteNotFound = errors.New("obsidian note not found")

type RESTClient struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

func NewRESTClient(baseURL, apiKey string, insecureSkipVerify bool) *RESTClient {
	if baseURL == "" {
		baseURL = "https://127.0.0.1:27124"
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if insecureSkipVerify {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec
	}
	return &RESTClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		client:  &http.Client{Timeout: 20 * time.Second, Transport: transport},
	}
}

func (c *RESTClient) Enabled() bool {
	return c != nil && c.apiKey != ""
}

func (c *RESTClient) Health(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/", nil)
	if err != nil {
		return err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("obsidian rest health status: %d", resp.StatusCode)
	}
	return nil
}

func (c *RESTClient) ReadNote(ctx context.Context, vaultPath string) ([]byte, error) {
	req, err := c.newVaultRequest(ctx, http.MethodGet, vaultPath, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, ErrNoteNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, fmt.Errorf("obsidian read status: %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func (c *RESTClient) WriteNote(ctx context.Context, vaultPath string, content []byte) error {
	req, err := c.newVaultRequest(ctx, http.MethodPut, vaultPath, bytes.NewReader(content))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "text/markdown; charset=utf-8")
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("obsidian write status: %d", resp.StatusCode)
	}
	return nil
}

func (c *RESTClient) newVaultRequest(ctx context.Context, method, vaultPath string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+"/vault/"+escapeVaultPath(vaultPath), body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "text/markdown, application/json")
	return req, nil
}

func escapeVaultPath(vaultPath string) string {
	parts := strings.Split(filepathSlash(vaultPath), "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}

func filepathSlash(path string) string {
	return strings.ReplaceAll(path, "\\", "/")
}
