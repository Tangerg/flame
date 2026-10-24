package main

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	runtimeclient "github.com/Tangerg/flame/packages/runtime-client/go"
	schedule "github.com/Tangerg/flame/plugins/schedules/backend/domain"
	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/protocol"
)

func TestRunAdmissionReleasesObservationAndRetainsOriginalIdentity(t *testing.T) {
	for _, outcome := range []string{"accepted", "wrong acknowledgement", "lost acknowledgement"} {
		t.Run(outcome, func(t *testing.T) {
			discovery, err := os.ReadFile("../../../runtime/contract/samples/method.discover.resp.json")
			if err != nil {
				t.Fatal(err)
			}
			var starts atomic.Int32
			closed := make(chan struct{}, 4)
			var mu sync.Mutex
			accepted := make(map[string]protocol.StartRunRequest)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					ID     string         `json:"id"`
					Method string         `json:"method"`
					Params jsontext.Value `json:"params"`
				}
				if err := json.UnmarshalRead(r.Body, &request); err != nil {
					t.Error(err)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				switch request.Method {
				case "runtime.discover":
					fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%q,"result":%s}`, request.ID, discovery)
				case "runs.getAcceptance":
					var query protocol.GetRunAcceptanceRequest
					if err := json.Unmarshal(request.Params, &query); err != nil {
						t.Error(err)
						return
					}
					mu.Lock()
					original, found := accepted[query.Key]
					mu.Unlock()
					if !found {
						fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%q,"result":{"type":"notRecorded"}}`, request.ID)
						return
					}
					if query.Operation != protocol.RunCommandStart || !reflect.DeepEqual(query.Request, &original) {
						t.Error("reconciliation changed the original command")
					}
					fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%q,"result":{"type":"retired","sessionId":%q,"runId":%q,"segmentId":"seg_original","userItemId":"item_original"}}`, request.ID, original.NewSession.SessionID, original.NewSession.RunID)
				case "runs.start":
					var start protocol.StartRunRequest
					if err := json.Unmarshal(request.Params, &start); err != nil || start.NewSession == nil {
						t.Errorf("missing captured opening: %v", err)
						return
					}
					starts.Add(1)
					mu.Lock()
					accepted[r.Header.Get("Idempotency-Key")] = start
					mu.Unlock()
					defer func() { closed <- struct{}{} }()
					w.Header().Set("Content-Type", "text/event-stream")
					if outcome == "lost acknowledgement" {
						w.WriteHeader(http.StatusOK)
						return
					}
					runID := start.NewSession.RunID
					if outcome == "wrong acknowledgement" {
						runID = "run_foreign"
					}
					fmt.Fprintf(w, "data: {\"jsonrpc\":\"2.0\",\"id\":%q,\"result\":{\"runId\":%q,\"segmentId\":\"seg_original\",\"userItemId\":\"item_original\"}}\n\n", request.ID, runID)
					if err := http.NewResponseController(w).Flush(); err != nil {
						t.Error(err)
						return
					}
					<-r.Context().Done()
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
			host := runHost{client: client}
			activation, cancel := context.WithCancel(t.Context())
			defer cancel()
			for index := range 4 {
				at := time.Unix(1700000000, 0).UTC()
				occurrence, err := schedule.RestoreOccurrence(schedule.OccurrenceSnapshot{
					ID: "sch_due:1700000000000", Execution: schedule.ExecutionSnapshot{Title: "Due", Instructions: "captured input"},
					FiredAt: at, NextRunAt: at.Add(time.Hour), SessionID: fmt.Sprintf("ses_due_%d", index), RunID: fmt.Sprintf("run_due_%d", index),
					Invocation: capability.Invocation{ID: fmt.Sprintf("trigger-%d", index), Origin: capability.Automatic, Target: capability.Target{Scope: capability.ScopeInstallation, Reference: "11111111-1111-4111-8111-111111111111"}},
				})
				if err != nil {
					t.Fatal(err)
				}
				request := occurrence.RunRequest()
				err = host.Admit(activation, request)
				switch outcome {
				case "accepted":
					if err != nil {
						t.Fatal(err)
					}
				case "wrong acknowledgement":
					if !errors.Is(err, protocol.ErrInvalidResponse) {
						t.Fatalf("wrong acknowledgement: %v", err)
					}
				case "lost acknowledgement":
					if !errors.Is(err, runtimeclient.ErrAcknowledgementUnknown) {
						t.Fatalf("lost acknowledgement became a definite result: %v", err)
					}
				}
				select {
				case <-closed:
				case <-time.After(5 * time.Second):
					t.Fatal("admission retained its observation after returning")
				}
				if activation.Err() != nil {
					t.Fatal("admission closed the plugin activation")
				}
				if err := host.Admit(activation, request); err != nil {
					t.Fatalf("original acceptance could not be recovered: %v", err)
				}
				if starts.Load() != int32(index+1) {
					t.Fatal("reconciliation dispatched another Run")
				}
			}
		})
	}
}
