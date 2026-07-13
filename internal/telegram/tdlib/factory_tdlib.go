//go:build tdlib && cgo

package tdlib

import (
	"context"
	"errors"
	"sync"
)

type NativeClientFactory struct {
	cfg     ClientConfig
	mu      sync.Mutex
	clients map[string]*NativeClient
}

func NewClientFactory(cfg ClientConfig) ClientFactory {
	return &NativeClientFactory{
		cfg:     cfg,
		clients: make(map[string]*NativeClient),
	}
}

func (f *NativeClientFactory) NewClient(sessionPath string) (TelegramClient, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	client, ok := f.clients[sessionPath]
	if ok {
		return client, nil
	}
	client = NewNativeClient(f.cfg, sessionPath)
	f.clients[sessionPath] = client
	return client, nil
}

func (f *NativeClientFactory) Close(ctx context.Context) error {
	f.mu.Lock()
	clients := make([]*NativeClient, 0, len(f.clients))
	for _, client := range f.clients {
		clients = append(clients, client)
	}
	f.clients = make(map[string]*NativeClient)
	f.mu.Unlock()

	var closeErr error
	for _, client := range clients {
		closeErr = errors.Join(closeErr, client.Stop(ctx))
	}
	return closeErr
}
