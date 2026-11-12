package client

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

func TestCommandHeaderValuesPreserveCallerIdentity(t *testing.T) {
	for _, value := range []string{"original\x00", "original\x01", "original\x7f", " original", "original ", "\toriginal", "original\t"} {
		for _, options := range []protocol.CommandOptions{
			{IdempotencyKey: value},
			{IdempotencyKey: "original", IdempotencyNamespace: value},
		} {
			t.Run(value, func(t *testing.T) {
				received := make(chan string, 1)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					received <- r.Header.Get("Idempotency-Key")
					w.WriteHeader(http.StatusBadRequest)
				}))
				defer server.Close()
				c, err := Connect(t.Context(), RemoteConfig{Endpoint: server.URL})
				if err != nil {
					t.Fatal(err)
				}
				defer c.Close()
				_, err = c.CreateSession(t.Context(), protocol.CreateSessionRequest{}, options)
				if !errors.Is(err, protocol.ErrInvalidParams) || errors.Is(err, ErrAcknowledgementUnknown) {
					t.Errorf("unsafe header must fail before dispatch with invalid_params, got %v", err)
				}
				select {
				case actual := <-received:
					t.Errorf("unsafe identity dispatched with key %q", actual)
				default:
				}
			})
		}
	}
}

func TestRemoteCredentialsMustBeExactHeaderValues(t *testing.T) {
	for _, token := range []string{"secret\x00suffix", "secret\x01suffix", "secret\x7fsuffix", "secret\rsuffix", "secret\nsuffix", " secret", "secret "} {
		t.Run(token, func(t *testing.T) {
			client, err := Connect(t.Context(), RemoteConfig{Endpoint: "https://runtime.example", Token: token})
			if client != nil {
				_ = client.Close()
			}
			if err == nil {
				t.Fatal("unrepresentable credential admitted")
			}
			if strings.Contains(err.Error(), token) {
				t.Fatal("credential appeared in a validation error")
			}
		})
	}
}

func TestRepresentableCommandHeaderValuesRemainExact(t *testing.T) {
	for _, key := range []string{"original", "original key", "clé", "原始身份"} {
		t.Run(key, func(t *testing.T) {
			received := make(chan string, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				received <- r.Header.Get("Idempotency-Key")
				w.WriteHeader(http.StatusBadRequest)
			}))
			defer server.Close()
			client, err := Connect(t.Context(), RemoteConfig{Endpoint: server.URL})
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			_, _ = client.CreateSession(t.Context(), protocol.CreateSessionRequest{}, protocol.CommandOptions{IdempotencyKey: key})
			select {
			case actual := <-received:
				if actual != key {
					t.Errorf("key = %q, want %q", actual, key)
				}
			default:
				t.Fatal("representable identity was refused before dispatch")
			}
		})
	}
}
