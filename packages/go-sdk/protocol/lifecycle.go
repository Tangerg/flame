package protocol

type DiscoverResponse struct {
	ProtocolVersion string             `json:"protocolVersion"`
	ServerInfo      ServerInfo         `json:"serverInfo"`
	Capabilities    ServerCapabilities `json:"capabilities"`
}

type ClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// InstanceID is fresh for every Bootstrap instance and must never be used as a
// durable storage or idempotency namespace.
type ServerInfo struct {
	InstanceID       string       `json:"instanceId"`
	Name             string       `json:"name"`
	Version          string       `json:"version"`
	DefaultWorkspace WorkspaceRef `json:"defaultWorkspace"`
	Home             string       `json:"home"`
}
