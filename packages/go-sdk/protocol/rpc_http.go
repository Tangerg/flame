package protocol

const (
	RPCCallStatus        = 200
	RPCDocumentMediaType = "application/json"
	RPCStreamMediaType   = "text/event-stream"
	HTTPProblemMediaType = "application/problem+json"
)

type HTTPProblem struct {
	Type   string `json:"type"`
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail"`
}
