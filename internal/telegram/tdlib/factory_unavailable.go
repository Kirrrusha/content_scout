//go:build !tdlib

package tdlib

func NewClientFactory(ClientConfig) ClientFactory {
	return UnavailableClientFactory{}
}
