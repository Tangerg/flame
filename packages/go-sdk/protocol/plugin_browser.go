package protocol

// OpenPluginBrowserRealmRequest preserves one bootstrap operation and its caller
// generated recovery secret. Resume names the original actual JavaScript Realm.
type OpenPluginBrowserRealmRequest struct {
	OperationID string              `json:"operationId"`
	ResumeToken string              `json:"resumeToken"`
	Resume      *PluginBrowserRealm `json:"resume,omitzero"`
}

type PluginBrowserSession struct {
	Authority                PluginBrowserAuthority   `json:"authority"`
	Realm                    PluginBrowserRealm       `json:"realm"`
	Credential               string                   `json:"credential"`
	ControlEndpoint          string                   `json:"controlEndpoint"`
	MaximumMessageBytes      int                      `json:"maximumMessageBytes"`
	ProofCapacity            int                      `json:"proofCapacity"`
	PollIntervalMilliseconds int                      `json:"pollIntervalMilliseconds"`
	ComponentContract        PluginBrowserContractRef `json:"componentContract"`
}

type AcquirePluginComponentRequest struct {
	PluginReleaseRequest
	OperationID string             `json:"operationId"`
	ParentUseID string             `json:"parentUseId,omitzero"`
	Realm       PluginBrowserRealm `json:"realm"`
	ComponentID string             `json:"componentId"`
}

type ClosePluginBrowserRealmRequest struct {
	Realm PluginBrowserRealm `json:"realm"`
}
