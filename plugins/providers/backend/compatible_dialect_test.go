package main

import (
	json "encoding/json/v2"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/plugin/modelprovider"
	"github.com/Tangerg/scope/core/chat"
	anthropicprotocol "github.com/Tangerg/scope/models/protocol/anthropic"
	openaiprotocol "github.com/Tangerg/scope/models/protocol/openai"
)

func TestCompatibleProvidersUseTheirOwnWireDialects(t *testing.T) {
	binary := buildProviderBackend(t)
	call := func(t *testing.T, provider Provider, endpoint string, customize func(*chat.Request)) {
		t.Helper()
		session, _ := connectProvider(t, binary, t.TempDir())
		opened := openChat(t, session, string(provider), endpoint)
		request := providerRequest(opened.Handle)
		customize(&request.Request)
		method, err := modelprovider.CallMethod()
		if err != nil {
			t.Fatal(err)
		}
		response, err := providerMethod(t, session, method).CallCommand(providerContext(t), "gateway-call", request)
		if err != nil || response.Failure != nil || response.Response == nil || response.Response.Output.Text() != "answer" {
			t.Fatalf("compatible exchange = %+v, %v", response, err)
		}
	}

	t.Run("Anthropic", func(t *testing.T) {
		const nativeSignature = "native-only-signature"
		const temperature = 0.4
		type observedRequest struct {
			Revision    string
			Signature   string
			Temperature *float64
			CacheType   string
		}
		observed := make(chan observedRequest, 1)
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
			var body struct {
				Temperature  *float64 `json:"temperature"`
				CacheControl struct {
					Type string `json:"type"`
				} `json:"cache_control"`
				Messages []struct {
					Content []struct {
						Type      string `json:"type"`
						Signature string `json:"signature"`
					} `json:"content"`
				} `json:"messages"`
			}
			if err := json.UnmarshalRead(request.Body, &body); err != nil || request.URL.Path != "/v1/messages" {
				t.Errorf("invalid Messages request path=%q: %v", request.URL.Path, err)
				http.Error(w, "invalid Messages request", http.StatusBadRequest)
				return
			}
			input := observedRequest{
				Revision: request.Header.Get("anthropic-version"), Temperature: body.Temperature, CacheType: body.CacheControl.Type,
			}
			for _, message := range body.Messages {
				for _, part := range message.Content {
					if part.Type == "thinking" {
						input.Signature = part.Signature
					}
				}
			}
			select {
			case observed <- input:
			default:
				t.Error("Messages exchange was repeated")
			}
			w.Header().Set("Content-Type", "text/event-stream")
			for _, event := range []struct {
				name string
				body string
			}{
				{"message_start", `{"type":"message_start","message":{"id":"msg-gateway","type":"message","role":"assistant","model":"model-test","content":[],"usage":{"input_tokens":5,"output_tokens":0}}}`},
				{"content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`},
				{"content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"answer"}}`},
				{"content_block_stop", `{"type":"content_block_stop","index":0}`},
				{"message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":2}}`},
				{"message_stop", `{"type":"message_stop"}`},
			} {
				_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.name, event.body)
			}
		}))
		t.Cleanup(upstream.Close)
		thinking, err := anthropicprotocol.NewThinkingPart("private native reasoning", []byte(nativeSignature))
		if err != nil {
			t.Fatal(err)
		}
		call(t, providerAnthropicCompatible, upstream.URL, func(request *chat.Request) {
			request.Options.Temperature = new(temperature)
			request.Messages = append(request.Messages, chat.NewAssistantMessage(thinking, chat.NewTextPart("previous answer")), chat.NewUserMessage(chat.NewTextPart("continue")))
		})
		select {
		case request := <-observed:
			if request.Signature != "" || request.CacheType != "" || request.Revision != string(anthropicprotocol.MessagesRevision20230601) || request.Temperature == nil || *request.Temperature != temperature {
				t.Fatalf("compatible gateway received an unexpected Messages request: %+v", request)
			}
		default:
			t.Fatal("gateway received no request")
		}
	})

	t.Run("OpenAI", func(t *testing.T) {
		const compatibleSeed = int64(7)
		const nativeSeed = int64(9)
		observed := make(chan int64, 1)
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
			var body struct {
				Seed int64 `json:"seed"`
			}
			if err := json.UnmarshalRead(request.Body, &body); err != nil || request.URL.Path != "/chat/completions" {
				t.Errorf("invalid Chat Completions request path=%q: %v", request.URL.Path, err)
				http.Error(w, "invalid Chat Completions request", http.StatusBadRequest)
				return
			}
			select {
			case observed <- body.Seed:
			default:
				t.Error("Chat Completions exchange was repeated")
			}
			writeChatReply(w)
		}))
		t.Cleanup(upstream.Close)
		call(t, providerOpenAICompatible, upstream.URL, func(request *chat.Request) {
			if err := request.Options.Extensions.Set(openaiprotocol.RequestExtensionKey, openaiprotocol.RequestFields{"seed": nativeSeed}); err != nil {
				t.Fatal(err)
			}
			if err := request.Options.Extensions.Set(string(providerOpenAICompatible)+openaiprotocol.CompatibleRequestExtensionSuffix, openaiprotocol.RequestFields{"seed": compatibleSeed}); err != nil {
				t.Fatal(err)
			}
		})
		select {
		case seed := <-observed:
			if seed != compatibleSeed {
				t.Fatalf("compatible gateway received seed %d, want %d", seed, compatibleSeed)
			}
		default:
			t.Fatal("gateway received no request")
		}
	})
}
