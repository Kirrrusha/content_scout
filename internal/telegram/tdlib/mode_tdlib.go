//go:build tdlib && cgo

package tdlib

func AdapterMode() string {
	return "native-tdjson"
}
