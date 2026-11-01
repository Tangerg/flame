package protocol

// Calls with an ID require an envelope. The endpoint also serves notifications,
// whose accepted/no-content statuses can never acknowledge a call.
const (
	RPCCallStatus        = 200
	RPCDocumentMediaType = "application/json"
	RPCStreamMediaType   = "text/event-stream"
	HTTPProblemMediaType = "application/problem+json"
)
