package dispatch

import "github.com/Tangerg/go-sdk/jsonrpc"

type StreamFrame struct {
	Notification jsonrpc.Message
	SSEID        string
}
