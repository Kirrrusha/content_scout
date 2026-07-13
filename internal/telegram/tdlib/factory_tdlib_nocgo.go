//go:build tdlib && !cgo

package tdlib

func NewClientFactory(ClientConfig) ClientFactory {
	return UnavailableClientFactory{}
}
