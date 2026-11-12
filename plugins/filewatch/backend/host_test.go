package main

import (
	json "encoding/json/v2"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	runtimeclient "github.com/Tangerg/flame/packages/go-sdk/client"
	"github.com/google/uuid"
)

func TestRunDispatchRetainsTheAcceptanceNamespace(t *testing.T) {
	const namespace = "idp_11111111111111111111111111111111"
	observation := captureFixture(t, 1, uuid.NewString())
	capture := observation.Capture()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ID     string `json:"id"`
			Method string `json:"method"`
			Params struct {
				Namespace string `json:"namespace"`
			} `json:"params"`
		}
		if err := json.UnmarshalRead(r.Body, &request); err != nil {
			t.Error(err)
			return
		}
		switch request.Method {
		case "runs.getAcceptance":
			if request.Params.Namespace != namespace {
				t.Error("acceptance lookup lost the original Runtime namespace")
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%q,"result":{"type":"notRecorded"}}`, request.ID)
		case "runs.start":
			if r.Header.Get("Idempotency-Namespace") != namespace {
				t.Error("Run dispatch lost the acceptance lookup's Runtime namespace")
			}
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprintf(w, "data: {\"jsonrpc\":\"2.0\",\"id\":%q,\"result\":{\"runId\":%q,\"segmentId\":\"seg_original\",\"userItemId\":\"item_original\"}}\n\n", request.ID, capture.RunID)
		default:
			t.Errorf("unexpected method %q", request.Method)
		}
	}))
	defer server.Close()
	client, err := runtimeclient.Connect(t.Context(), runtimeclient.RemoteConfig{Endpoint: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	host := &runHost{client: client}
	if err := host.Admit(t.Context(), namespace, observation); err != nil {
		t.Fatal(err)
	}
}
