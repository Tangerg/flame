package main

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"context"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/modelprovider"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/models/deepseek"
	"github.com/Tangerg/scope/models/google"
)

func mustClientSpec(t testing.TB, provider Provider, key, endpoint string) clientSpec {
	t.Helper()
	spec, err := newClientSpec(modelprovider.Connection{Provider: string(provider), APIKey: key, BaseURL: endpoint})
	if err != nil {
		t.Fatal(err)
	}
	return spec
}
func buildChatForTest(ctx context.Context, spec clientSpec, modelID string) (chat.Model, inputTokenCounter, error) {
	model, err := buildModel(ctx, spec, modelID)
	counter, _ := model.(inputTokenCounter)
	return model, counter, err
}
func TestClientSpecKeepsCredentialRedacted(t *testing.T) {
	const secret = "private-credential-fixture"
	spec := mustClientSpec(t, providerOpenAI, secret, "")
	for _, format := range []string{"%v", "%+v", "%#v"} {
		if strings.Contains(fmt.Sprintf(format, spec), secret) {
			t.Errorf("ClientSpec exposes its credential through %s", format)
		}
	}
}

// TestChatProviderCatalogSatisfiesConstructionContract holds the constructed
// catalog to its contract. Invalid combinations cannot be registered.

func TestPerplexityUsesScopeResponsesAndEndpointCatalog(t *testing.T) {
	profile, _ := providers.lookup(providerPerplexity)
	if profile.profile.DefaultModel != "" || profile.profile.ModelSource != modelprovider.EndpointModels || profile.profile.EndpointRequired {
		t.Fatal("Perplexity retained a retired default or lost its provider-owned endpoint")
	}
	for _, streaming := range []bool{false, true} {
		t.Run(fmt.Sprintf("streaming=%t", streaming), func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				calls.Add(1)
				if request.Method != http.MethodPost || request.URL.Path != "/responses" || request.Header.Get("Authorization") != "Bearer test-key" {
					t.Errorf("unexpected Perplexity request: %s %s", request.Method, request.URL.Path)
				}
				var body struct {
					Model    string           `json:"model"`
					Input    []jsontext.Value `json:"input"`
					Messages []jsontext.Value `json:"messages"`
				}
				if err := json.UnmarshalRead(request.Body, &body); err != nil || body.Model != "openai/gpt-6-sol" || len(body.Input) != 1 || body.Messages != nil {
					t.Errorf("unexpected Responses input: %+v, %v", body, err)
				}
				writer.Header().Set("Content-Type", "text/event-stream")
				_, _ = fmt.Fprint(writer, "data: {\"type\":\"response.output_text.delta\",\"item_id\":\"message-1\",\"output_index\":0,\"content_index\":0,\"delta\":\"answer\"}\n\n")
				_, _ = fmt.Fprint(writer, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"response-1\",\"model\":\"openai/gpt-6-sol\",\"status\":\"completed\",\"output\":[{\"type\":\"message\",\"id\":\"message-1\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"answer\"}]}],\"usage\":{\"input_tokens\":0,\"output_tokens\":2,\"total_tokens\":2}}}\n\n")
			}))
			t.Cleanup(server.Close)
			model, counter, err := buildChatForTest(t.Context(), mustClientSpec(t, providerPerplexity, "test-key", server.URL), "openai/gpt-6-sol")
			if err != nil || counter != nil {
				t.Fatalf("Perplexity construction = %v, counter=%v", err, counter)
			}
			request, _ := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("question")))
			var response *chat.Response
			if streaming {
				var accumulated chat.ResponseAccumulator
				for delta, cause := range model.(chat.Streamer).Stream(t.Context(), request) {
					if cause != nil {
						t.Fatal(cause)
					}
					if err := accumulated.Add(delta); err != nil {
						t.Fatal(err)
					}
				}
				response, err = accumulated.Response()
			} else {
				response, err = model.Call(t.Context(), request)
			}
			if err != nil || response == nil || response.Text() != "answer" || response.Metadata.Usage.InputTokens != 0 || response.Metadata.Usage.OutputTokens != 2 || calls.Load() != 1 {
				t.Fatalf("Perplexity response = %+v, %v; calls=%d", response, err, calls.Load())
			}
		})
	}
}

func TestBuildChatDeepSeekReasoningSurvivesOrdinarySecondTurn(t *testing.T) {
	var calls atomic.Int32
	var secondRequest struct {
		Messages []map[string]any `json:"messages"`
		Stream   bool             `json:"stream"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		call := calls.Add(1)
		if call == 2 {
			if err := json.UnmarshalRead(request.Body, &secondRequest); err != nil {
				t.Errorf("decode second request: %v", err)
				http.Error(writer, "invalid request", http.StatusBadRequest)
				return
			}
		}
		writer.Header().Set("Content-Type", "text/event-stream")
		if call == 1 {
			_, _ = writer.Write([]byte(`data: {"id":"first","model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"role":"assistant","reasoning_content":"private chain","content":"first answer"},"finish_reason":"stop"}]}` + "\n\ndata: [DONE]\n\n"))
			return
		}
		_, _ = writer.Write([]byte(`data: {"id":"second","model":"deepseek-v4-flash","choices":[{"index":0,"delta":{"role":"assistant","content":"second answer"},"finish_reason":"stop"}]}` + "\n\ndata: [DONE]\n\n"))
	}))
	t.Cleanup(server.Close)

	client, _, err := buildChatForTest(t.Context(), mustClientSpec(t, providerDeepSeek, "test-key", server.URL), deepseek.ModelFlash)
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
	if !secondRequest.Stream {
		t.Fatal("complete call did not use streaming transport")
	}
	assistant := findWireAssistant(t, secondRequest.Messages)
	if assistant["reasoning_content"] != "private chain" {
		t.Fatalf("ordinary prior turn lost provider-owned reasoning: %#v", assistant)
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

	client, _, err := buildChatForTest(t.Context(), mustClientSpec(t, providerDeepSeek, "test-key", server.URL), deepseek.ModelFlash)
	if err != nil {
		t.Fatalf("BuildChat: %v", err)
	}
	request, err := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("trigger rate limit")))
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Call(t.Context(), request)
	result := failure(err)
	if result == nil || result.Kind != modelprovider.RateLimited || result.RetryAfterMillis != 1 {
		t.Fatalf("failure = %+v, want rate limited with 1ms retry", result)
	}

}

func TestBuildChatGoogleUsesConfiguredEndpoint(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		if !strings.Contains(request.URL.Path, ":streamGenerateContent") {
			t.Errorf("Google request path = %q, want streamGenerateContent", request.URL.Path)
		}
		writer.Header().Set("Content-Type", "text/event-stream")
		_, _ = writer.Write([]byte(`data: {"responseId":"custom-endpoint","modelVersion":"gemini-3.6-flash","candidates":[{"index":0,"content":{"role":"model","parts":[{"text":"custom endpoint"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":2,"totalTokenCount":3}}` + "\n\n"))
	}))
	t.Cleanup(server.Close)

	client, _, err := buildChatForTest(t.Context(), mustClientSpec(t, providerGoogle, "test-key", server.URL), google.ModelGemini36Flash)
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
			Model    string           `json:"model"`
			Messages []jsontext.Value `json:"messages"`
		}
		if err := json.UnmarshalRead(request.Body, &body); err != nil {
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

	_, counter, err := buildChatForTest(t.Context(), mustClientSpec(t, providerAnthropic, "test-key", server.URL), "claude-test")
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
			writer.Header().Set("Content-Type", "text/event-stream")
			_, _ = writer.Write([]byte(`event: response.output_text.delta
data: {"type":"response.output_text.delta","sequence_number":0,"item_id":"msg_flame","output_index":0,"content_index":0,"delta":"done","logprobs":[]}

event: response.completed
data: {"type":"response.completed","sequence_number":1,"response":{"id":"resp_flame","object":"response","created_at":1,"status":"completed","model":"gpt-5.6-sol","output":[{"type":"message","id":"msg_flame","status":"completed","role":"assistant","content":[{"type":"output_text","text":"done","annotations":[]}]}],"parallel_tool_calls":false,"tools":[],"usage":{"input_tokens":73,"output_tokens":1,"total_tokens":74,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}}}` + "\n\n"))
		default:
			t.Errorf("unexpected path %q", request.URL.Path)
			http.Error(writer, "unexpected path", http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	direct, counter, err := buildChatForTest(t.Context(), mustClientSpec(t, providerOpenAI, "test-key", server.URL), defaultOpenAIModel)
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

	_, counter, err = buildChatForTest(t.Context(), mustClientSpec(t, providerOpenAICompatible, "test-key", "https://gateway.example/v1"), "compatible-model")
	if err != nil {
		t.Fatal(err)
	}
	if counter != nil {
		t.Fatal("OpenAI-compatible client advertised the native Responses count endpoint")
	}
}
