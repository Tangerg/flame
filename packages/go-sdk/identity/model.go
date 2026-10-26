package identity

const (
	// MaximumProviderCharacters leaves substantial headroom over the bundled
	// catalog's longest provider id while keeping durable keys bounded.
	MaximumProviderCharacters = 64
	// MaximumModelCharacters accommodates namespaced compatible model ids and
	// deployment names without allowing endpoint-controlled megabyte ids.
	MaximumModelCharacters = 256
	// MaximumReasoningEffortCharacters accommodates provider vocabularies while
	// keeping a model-owned option an identity rather than arbitrary prose.
	MaximumReasoningEffortCharacters = 32
)
