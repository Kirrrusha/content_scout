package bot

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"

	"golang.org/x/net/proxy"
)

// newHTTPClient builds an http.Client for talking to the Telegram Bot API.
// When proxyURL is empty it returns the same client tgbotapi.NewBotAPI uses
// by default. Otherwise it routes all requests through the given proxy
// (e.g. socks5://user:pass@host:port), needed when api.telegram.org is
// unreachable from the deployment region.
func newHTTPClient(proxyURL string) (*http.Client, error) {
	if proxyURL == "" {
		return &http.Client{}, nil
	}
	parsed, err := url.Parse(proxyURL)
	if err != nil {
		return nil, fmt.Errorf("parse telegram proxy url: %w", err)
	}
	dialer, err := proxy.FromURL(parsed, proxy.Direct)
	if err != nil {
		return nil, fmt.Errorf("build telegram proxy dialer: %w", err)
	}
	contextDialer, ok := dialer.(proxy.ContextDialer)
	if !ok {
		return nil, fmt.Errorf("telegram proxy dialer does not support context dialing")
	}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return contextDialer.DialContext(ctx, network, addr)
		},
	}
	return &http.Client{Transport: transport}, nil
}
