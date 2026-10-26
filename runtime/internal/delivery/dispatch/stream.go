package dispatch

import "github.com/Tangerg/go-sdk/jsonrpc"

// StreamFrame is one ready-to-write downstream notification on a streaming
// method's event stream. The dispatch produces these from domain events so
// every transport writes them identically. SSEID drives Last-Event-Id replay;
// "" marks an ephemeral frame (no replay) — e.g. all workspace events.
type StreamFrame struct {
	Notification jsonrpc.Message
	SSEID        string
}
