package llm

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/models/deepseek"
	"github.com/Tangerg/scope/models/google"
)

func mustClientSpec(t testing.TB, provider Provider, model, apiKey, baseURL string) ClientSpec {
	t.Helper()
	credential := NoClientCredential()
	if apiKey != "" {
		var err error
		credential, err = NewAPIKeyCredential(apiKey)
		if err != nil {
			t.Fatal(err)
		}
	}
	spec, err := NewClientSpec(provider, model, credential)
	if err != nil {
		t.Fatal(err)
	}
	if baseURL != "" {
		spec, err = spec.WithBaseURL(baseURL)
		if err != nil {
			t.Fatal(err)
		}
	}
	return spec
}

// TestChatProviderCatalogSatisfiesConstructionContract holds the constructed
// catalog to its contract. Invalid combinations cannot be registered.
func TestChatProviderCatalogSatisfiesConstructionContract(t *testing.T) {
	for _, provider := range providers.supported() {
		profile, found := providers.lookup(provider)
		if !found {
			t.Fatalf("provider %q disappeared from its own catalog", provider)
		}
		if profile.chatBuilder == nil {
			t.Errorf("provider %q: nil build func", provider)
		}
		if profile.credential.environment == "" {
			t.Errorf("provider %q: empty apiKeyEnv", provider)
		}
	}

	// The compatible endpoint providers and Azure carry no built-in endpoint.
	for _, p := range []Provider{ProviderOpenAICompatible, ProviderAnthropicCompatible, ProviderAzureOpenAI} {
		profile, found := LookupProvider(p)
		if !found || !profile.RequiresConfiguredEndpoint() {
			t.Errorf("provider %q must require a base URL", p)
		}
	}
	// A named vendor must NOT require one (it has a built-in endpoint).
	anthropic, _ := LookupProvider(ProviderAnthropic)
	if anthropic.RequiresConfiguredEndpoint() {
		t.Error("anthropic must not require a base URL")
	}
}

func TestProviderCatalogRejectsContradictoryProfiles(t *testing.T) {
	valid := bundledProvider(ProviderOpenAI, defaultOpenAIModel, "OPENAI_API_KEY", buildOpenAIResponsesModel)
	cases := []struct {
		name     string
		profiles []providerProfile
		want     string
	}{
		{name: "duplicate identity", profiles: []providerProfile{valid, valid}, want: "more than once"},
		{name: "blank bundled default", profiles: []providerProfile{bundledProvider("broken", "", "BROKEN_API_KEY", buildOpenAIResponsesModel)}, want: "requires a default model"},
		{name: "invalid provider identity", profiles: []providerProfile{bundledProvider("broken\x00shadow", "model", "BROKEN_API_KEY", buildOpenAIResponsesModel)}, want: "provider identity"},
		{name: "invalid bundled default", profiles: []providerProfile{bundledProvider("broken", "model\x00shadow", "BROKEN_API_KEY", buildOpenAIResponsesModel)}, want: "model identity"},
		{name: "non-canonical environment", profiles: []providerProfile{bundledProvider("broken", "model", "broken_api_key", buildOpenAIResponsesModel)}, want: "not canonical"},
		{name: "endpoint discovery without endpoint", profiles: []providerProfile{endpointProvider("broken", adapterEndpoint(), "BROKEN_API_KEY", buildOpenAIResponsesModel)}, want: "resolvable endpoint"},
		{name: "required endpoint carrying default", profiles: []providerProfile{{
			id: "broken", credential: requiredCredential("BROKEN_API_KEY"),
			endpoint:   endpointPolicy{kind: endpointMustBeConfigured, defaultURL: "https://example.test"},
			chatModels: openAIEndpointModels(), chatBuilder: buildOpenAIResponsesModel,
		}}, want: "cannot carry a default URL"},
		{name: "embedding without model policy", profiles: []providerProfile{valid.withEmbedding(modelPolicy{}, buildOpenAIEmbeddingModel)}, want: "embedding"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if _, err := newProviderCatalog(test.profiles...); err == nil {
				t.Fatal("newProviderCatalog accepted contradictory provider state")
			} else if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("newProviderCatalog error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestBuildChatDeepSeekReasoningSurvivesOrdinarySecondTurn(t *testing.T) {
	var calls atomic.Int32
	var secondRequest struct {
		Messages []map[string]any `json:"messages"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		call := calls.Add(1)
		if call == 2 {
			if err := json.NewDecoder(request.Body).Decode(&secondRequest); err != nil {
				t.Errorf("decode second request: %v", err)
				http.Error(writer, "invalid request", http.StatusBadRequest)
				return
			}
		}
		writer.Header().Set("Content-Type", "application/json")
		if call == 1 {
			_, _ = writer.Write([]byte(`{"id":"first","model":"deepseek-v4-flash","choices":[{"index":0,"message":{"role":"assistant","reasoning_content":"private chain","content":"first answer"},"finish_reason":"stop"}]}`))
			return
		}
		_, _ = writer.Write([]byte(`{"id":"second","model":"deepseek-v4-flash","choices":[{"index":0,"message":{"role":"assistant","content":"second answer"},"finish_reason":"stop"}]}`))
	}))
	t.Cleanup(server.Close)

	client, _, err := BuildChat(mustClientSpec(t, ProviderDeepSeek, deepseek.ModelV4Flash, "test-key", server.URL))
	if err != nil {
		t.Fatalf("BuildChat: %v", err)
	}
	firstUser := chat.NewUserMessage(chat.NewTextPart("first"))
	first, err := client.Call(t.Context(), &chat.Request{Messages: []chat.Message{firstUser}})
	if err != nil {
		t.Fatalf("first Call: %v", err)
	}
	if first.Output == nil || first.Output.Message == nil || len(first.Output.Message.Parts) != 2 || first.Output.Message.Parts[0].Kind != chat.PartReasoning {
		t.Fatalf("first response did not preserve reasoning: %#v", first)
	}

	second, err := client.Call(t.Context(), &chat.Request{Messages: []chat.Message{
		firstUser,
		*first.Output.Message,
		chat.NewUserMessage(chat.NewTextPart("second")),
	}})
	if err != nil {
		t.Fatalf("second Call: %v", err)
	}
	if second.Text() != "second answer" {
		t.Fatalf("second response text = %q", second.Text())
	}
	assistant := findWireAssistant(t, secondRequest.Messages)
	if _, exists := assistant["reasoning_content"]; exists {
		t.Fatalf("ordinary prior turn replayed reasoning_content: %#v", assistant)
	}
}

func TestBuildChatDeepSeekClassifiesRateLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.Header().Set("Retry-After", "12")
		writer.Header().Set("Retry-After-Ms", "1")
		writer.WriteHeader(http.StatusTooManyRequests)
		_, _ = writer.Write([]byte(`{"error":{"message":"slow down","type":"rate_limit_error","code":"rate_limit"}}`))
	}))
	t.Cleanup(server.Close)

	client, _, err := BuildChat(mustClientSpec(t, ProviderDeepSeek, deepseek.ModelV4Flash, "test-key", server.URL))
	if err != nil {
		t.Fatalf("BuildChat: %v", err)
	}
	request, err := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("trigger rate limit")))
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Call(t.Context(), request)
	var failure *run.FailureError
	if !errors.As(err, &failure) {
		t.Fatalf("Call error = %T, want *run.FailureError", err)
	}
	if failure.Kind != run.FailureRateLimited || failure.RetryAfter != time.Millisecond {
		t.Fatalf("failure = %+v, want rate limited with 1ms retry", failure)
	}
}

func TestBuildChatGoogleUsesConfiguredEndpoint(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		if !strings.Contains(request.URL.Path, ":generateContent") {
			t.Errorf("Google request path = %q, want generateContent", request.URL.Path)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{
  "responseId":"custom-endpoint","modelVersion":"gemini-3.6-flash",
  "candidates":[{"index":0,"content":{"role":"model","parts":[{"text":"custom endpoint"}]},"finishReason":"STOP"}],
  "usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":2,"totalTokenCount":3}
}`))
	}))
	t.Cleanup(server.Close)

	client, _, err := BuildChat(mustClientSpec(t, ProviderGoogle, google.ModelGemini36Flash, "test-key", server.URL))
	if err != nil {
		t.Fatalf("BuildChat: %v", err)
	}
	request, err := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("route this request")))
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Call(t.Context(), request)
	if err != nil {
		t.Fatalf("Call through configured endpoint: %v", err)
	}
	if response.Text() != "custom endpoint" || requests.Load() != 1 {
		t.Fatalf("configured endpoint response = %q; requests = %d", response.Text(), requests.Load())
	}
}

func findWireAssistant(t *testing.T, messages []map[string]any) map[string]any {
	t.Helper()
	for _, message := range messages {
		if message["role"] == "assistant" {
			return message
		}
	}
	t.Fatal("assistant message not found")
	return nil
}

// TestQueries covers the table-reader API providers.list / config.Load lean on.
func TestQueries(t *testing.T) {
	if got := len(SupportedProviders()); got != 21 {
		t.Errorf("SupportedProviders = %d, want 21", got)
	}
	if _, found := LookupProvider(ProviderGroq); !found {
		t.Error("groq should be supported")
	}
	if _, found := LookupProvider(Provider("nope")); found {
		t.Error("unknown provider should not be supported")
	}
	anthropic, _ := LookupProvider(ProviderAnthropic)
	if _, found := anthropic.DefaultChatModel(); !found {
		t.Error("anthropic should have a default model")
	}
	// A generic compatible endpoint has no catalog default — the model id is user-supplied.
	compatible, _ := LookupProvider(ProviderOpenAICompatible)
	if _, found := compatible.DefaultChatModel(); found {
		t.Error("openai-compatible should have no default model")
	}
	openAI, _ := LookupProvider(ProviderOpenAI)
	if openAI.CredentialEnvironment() != "OPENAI_API_KEY" {
		t.Errorf("openai key env = %q", openAI.CredentialEnvironment())
	}
}

// TestBuildChat covers the construction guards + a successful build (the
// adapter constructs a client without touching the network — no key validation
// until a call is made).
func TestBuildChat(t *testing.T) {
	// Unknown provider → error.
	if _, err := NewClientSpec("nope", "x", NoClientCredential()); err == nil {
		t.Error("unknown provider must error")
	}
	// A requiresBaseURL provider without a base URL → error naming the gap.
	if _, _, err := BuildChat(mustClientSpec(t, ProviderOpenAICompatible, "x", "k", "")); err == nil {
		t.Error("openai-compatible without base URL must error")
	} else if !strings.Contains(err.Error(), "base URL") {
		t.Errorf("error should mention the base URL: %v", err)
	}
	// A named vendor builds a non-nil client.
	c, _, err := BuildChat(mustClientSpec(t, ProviderAnthropic, "claude-3-5-haiku-20241022", "test-key", ""))
	if err != nil || c == nil {
		t.Fatalf("build anthropic: client=%v err=%v", c, err)
	}
	// A requiresBaseURL provider WITH a base URL builds.
	if _, _, err := BuildChat(mustClientSpec(t, ProviderOpenAICompatible, "x", "k", "https://gateway.example.com/v1")); err != nil {
		t.Errorf("openai-compatible with base URL: %v", err)
	}
}

func TestClientSpecRejectsPrimitiveSentinelsAndPartialState(t *testing.T) {
	credential, err := NewAPIKeyCredential(" key with exact surrounding whitespace ")
	if err != nil {
		t.Fatal(err)
	}
	if credential.sdkAPIKey() != " key with exact surrounding whitespace " {
		t.Fatal("API key material was normalized")
	}
	if _, err := NewClientSpec(ProviderOpenAI, " ", credential); err == nil {
		t.Fatal("blank model identity was accepted")
	}
	if _, err := NewClientSpec(ProviderOpenAI, "model\x00shadow", credential); err == nil {
		t.Fatal("non-printing model identity was accepted")
	}
	if _, err := NewClientSpec(ProviderOpenAI, defaultOpenAIModel, ClientCredential{}); err == nil {
		t.Fatal("zero credential state was accepted")
	}
	if _, err := NewClientSpec(ProviderOpenAI, defaultOpenAIModel, NoClientCredential()); err == nil {
		t.Fatal("required API key absence was accepted")
	}
	if _, err := NewAPIKeyCredential("\t \n"); err == nil {
		t.Fatal("blank API key was accepted")
	}

	spec, err := NewClientSpec(ProviderOpenAI, defaultOpenAIModel, credential)
	if err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{"", " https://example.test", "ftp://example.test", "https://user@example.test", "https://example.test/#fragment"} {
		if _, err := spec.WithBaseURL(invalid); err == nil {
			t.Errorf("base URL %q was accepted", invalid)
		}
	}
	if _, _, err := BuildChat(ClientSpec{}); err == nil {
		t.Fatal("zero ClientSpec was accepted")
	}
}

func TestProviderEndpointPolicyResolvesCatalogDefaultOnce(t *testing.T) {
	ollama, found := providers.lookup(ProviderOllama)
	if !found {
		t.Fatal("ollama profile is missing")
	}
	endpoint, err := ollama.endpoint.resolve(noClientEndpoint())
	if err != nil {
		t.Fatal(err)
	}
	if endpoint.sdkBaseURL() != defaultOllamaOpenAIBaseURL {
		t.Fatalf("resolved endpoint = %q, want %q", endpoint.sdkBaseURL(), defaultOllamaOpenAIBaseURL)
	}
	if _, _, err := BuildChat(mustClientSpec(t, ProviderOllama, "local-model", "", "")); err != nil {
		t.Fatalf("catalog-default Ollama client: %v", err)
	}
	ollamaProfile, _ := LookupProvider(ProviderOllama)
	openAIProfile, _ := LookupProvider(ProviderOpenAI)
	if ollamaProfile.RequiresAPIKey() || !openAIProfile.RequiresAPIKey() {
		t.Fatal("provider credential requirements are inverted")
	}
}

func TestDirectAnthropicExposesNativeInputTokenCounting(t *testing.T) {
	var countRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if !strings.HasSuffix(request.URL.Path, "/messages/count_tokens") {
			t.Errorf("unexpected path %q", request.URL.Path)
			http.Error(writer, "unexpected path", http.StatusNotFound)
			return
		}
		countRequests.Add(1)
		var body struct {
			Model    string            `json:"model"`
			Messages []json.RawMessage `json:"messages"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode count request: %v", err)
			http.Error(writer, "invalid request", http.StatusBadRequest)
			return
		}
		if body.Model != "claude-test" || len(body.Messages) != 1 {
			t.Errorf("count request = %#v", body)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"input_tokens":61}`))
	}))
	t.Cleanup(server.Close)

	_, counter, err := BuildChat(mustClientSpec(t, ProviderAnthropic, "claude-test", "test-key", server.URL))
	if err != nil {
		t.Fatal(err)
	}
	if counter == nil {
		t.Fatal("direct Anthropic client did not expose native input token counting")
	}
	request, err := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("measure me")))
	if err != nil {
		t.Fatal(err)
	}
	count, err := counter.CountInputTokens(t.Context(), request)
	if err != nil || count != 61 || countRequests.Load() != 1 {
		t.Fatalf("direct CountInputTokens = %d, %v; requests=%d", count, err, countRequests.Load())
	}
}

func TestDirectOpenAIUsesResponsesCountingWhileCompatibleRemainsChatCompletions(t *testing.T) {
	var countRequests atomic.Int32
	var responseRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/responses/input_tokens":
			countRequests.Add(1)
			_, _ = writer.Write([]byte(`{"object":"response.input_tokens","input_tokens":73}`))
		case "/responses":
			responseRequests.Add(1)
			_, _ = writer.Write([]byte(`{
  "id":"resp_flame","object":"response","created_at":1,"status":"completed","model":"gpt-5.6-sol",
  "output":[{"type":"message","id":"msg_flame","status":"completed","role":"assistant","content":[{"type":"output_text","text":"done","annotations":[]}]}],
  "parallel_tool_calls":false,"tools":[],
  "usage":{"input_tokens":73,"output_tokens":1,"total_tokens":74,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}
}`))
		default:
			t.Errorf("unexpected path %q", request.URL.Path)
			http.Error(writer, "unexpected path", http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	direct, counter, err := BuildChat(mustClientSpec(t, ProviderOpenAI, defaultOpenAIModel, "test-key", server.URL))
	if err != nil {
		t.Fatal(err)
	}
	if counter == nil {
		t.Fatal("direct OpenAI client did not expose Responses input token counting")
	}
	request, err := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("measure me")))
	if err != nil {
		t.Fatal(err)
	}
	count, err := counter.CountInputTokens(t.Context(), request)
	if err != nil || count != 73 || countRequests.Load() != 1 {
		t.Fatalf("direct CountInputTokens = %d, %v; requests=%d", count, err, countRequests.Load())
	}
	response, err := direct.Call(t.Context(), request)
	if err != nil || response.Text() != "done" || responseRequests.Load() != 1 {
		t.Fatalf("direct Responses Call = %#v, %v; requests=%d", response, err, responseRequests.Load())
	}

	_, counter, err = BuildChat(mustClientSpec(t, ProviderOpenAICompatible, "compatible-model", "test-key", "https://gateway.example/v1"))
	if err != nil {
		t.Fatal(err)
	}
	if counter != nil {
		t.Fatal("OpenAI-compatible client advertised the native Responses count endpoint")
	}
}
