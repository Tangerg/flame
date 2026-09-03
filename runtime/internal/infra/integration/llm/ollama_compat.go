package llm

import (
	"net/http"
	"strings"

	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/embedding"
	openaiprotocol "github.com/Tangerg/scope/models/protocol/openai"
)

const (
	ollamaProtocolProvider     = "ollama"
	ollamaSDKValidationAPIKey  = "flame-ollama-unauthenticated"
	defaultOllamaOpenAIBaseURL = "http://127.0.0.1:11434/v1"
)

// buildOllamaChatModel uses the daemon's supported OpenAI-compatible surface.
// Keeping the protocol adapter provider-scoped preserves ollama/* extension
// ownership without importing Ollama's server repository into the Runtime.
func buildOllamaChatModel(spec ClientSpec, opts chat.Options) (chat.Model, error) {
	apiKey, httpClient := ollamaProtocolAuthentication(spec)
	return openaiprotocol.NewCompatibleChatCompletions(openaiprotocol.ChatCompletionsConfig{
		APIKey:         apiKey,
		DefaultOptions: opts,
		BaseURL:        ollamaOpenAIBaseURL(spec.sdkBaseURL()),
		HTTPClient:     httpClient,
	}, openaiprotocol.Dialect{
		Provider: ollamaProtocolProvider, TokenLimitField: openaiprotocol.TokenLimitMaxTokens,
	})
}

// buildOllamaEmbeddingModel uses /v1/embeddings for the same reason as chat:
// Runtime needs a client protocol, not Ollama's model-management server module.
func buildOllamaEmbeddingModel(spec ClientSpec, opts embedding.Options) (embedding.Model, error) {
	apiKey, httpClient := ollamaProtocolAuthentication(spec)
	return openaiprotocol.NewEmbeddingModel(openaiprotocol.EmbeddingModelConfig{
		Provider:       ollamaProtocolProvider,
		APIKey:         apiKey,
		DefaultOptions: opts,
		BaseURL:        ollamaOpenAIBaseURL(spec.sdkBaseURL()),
		HTTPClient:     httpClient,
	})
}

func ollamaProtocolAuthentication(spec ClientSpec) (string, *http.Client) {
	if apiKey := spec.sdkAPIKey(); apiKey != "" {
		return apiKey, spec.sdkHTTPClient()
	}
	// scope's OpenAI protocol constructor requires a non-empty key even when the
	// target daemon does not authenticate. Satisfy that construction invariant,
	// then remove the header in this anti-corruption boundary so no fabricated
	// credential crosses the process boundary.
	client := *spec.sdkHTTPClient()
	client.Transport = authorizationStrippingRoundTripper{base: client.Transport}
	return ollamaSDKValidationAPIKey, &client
}

type authorizationStrippingRoundTripper struct {
	base http.RoundTripper
}

func (r authorizationStrippingRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	cloned := request.Clone(request.Context())
	cloned.Header = request.Header.Clone()
	cloned.Header.Del("Authorization")
	return r.base.RoundTrip(cloned)
}

func ollamaOpenAIBaseURL(configured string) string {
	baseURL := strings.TrimRight(configured, "/")
	if strings.HasSuffix(baseURL, "/v1") {
		return baseURL
	}
	return baseURL + "/v1"
}
