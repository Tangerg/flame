package main

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAssetRequestsRecordEffectsEvenWhenTheResourceIsMissing(t *testing.T) {
	probe := newProbe()
	handler, err := probe.assetHandler("wails://localhost/frame.html")
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		path     string
		status   int
		requests uint64
	}{
		{path: "/", status: http.StatusOK, requests: 0},
		{path: "/carrier-leak", status: http.StatusNotFound, requests: 1},
		{path: "/carrier-escape", status: http.StatusNotFound, requests: 2},
		{path: "/carrier-navigation", status: http.StatusNotFound, requests: 3},
	} {
		t.Run(tt.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, tt.path, nil))
			if response.Code != tt.status {
				t.Fatalf("status = %d, want %d", response.Code, tt.status)
			}
			if got := probe.snapshot().ResourceRequests; got != tt.requests {
				t.Fatalf("resource requests = %d, want %d", got, tt.requests)
			}
		})
	}
}

func TestCarrierEntryReceivesPolicyWithoutRestrictingHost(t *testing.T) {
	probe := newProbe()
	handler, err := probe.assetHandler("wails://localhost/frame.html")
	if err != nil {
		t.Fatal(err)
	}
	policy, err := assets.ReadFile("network-policy.txt")
	if err != nil {
		t.Fatal(err)
	}
	frame := httptest.NewRecorder()
	handler.ServeHTTP(frame, httptest.NewRequest(http.MethodGet, "/frame.html", nil))
	if frame.Code != http.StatusOK || frame.Header().Get("Connection-Allowlist") != strings.TrimSpace(string(policy)) {
		t.Fatalf("frame did not receive its enforcing policy: %d, %v", frame.Code, frame.Header())
	}
	host := httptest.NewRecorder()
	handler.ServeHTTP(host, httptest.NewRequest(http.MethodGet, "/", nil))
	if host.Header().Get("Connection-Allowlist") != "" {
		t.Fatal("guest policy restricted the trusted host")
	}
	if !strings.Contains(host.Body.String(), "frame-src wails://localhost/frame.html blob:") {
		t.Fatal("host frame policy did not name its constrained entry")
	}
	if probe.snapshot().ResourceRequests != 0 {
		t.Fatal("trusted carrier setup counted as a plugin escape")
	}
}

func TestPeerWitnessRecordsPacketsOutsideTheFrame(t *testing.T) {
	probe := newProbe()
	closePeer, err := probe.listenPeer()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := closePeer(); err != nil {
			t.Error(err)
		}
	})
	address := strings.TrimPrefix(probe.snapshot().PeerEndpoint, "stun:")
	sender, err := net.Dial("udp4", address)
	if err != nil {
		t.Fatal(err)
	}
	defer sender.Close()
	if _, err := sender.Write([]byte("positive control")); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for probe.snapshot().PeerPackets == 0 {
		if time.Now().After(deadline) {
			t.Fatal("peer witness did not observe its positive control")
		}
		time.Sleep(time.Millisecond)
	}
	if probe.snapshot().PeerPackets != 1 {
		t.Fatalf("peer packet count = %d", probe.snapshot().PeerPackets)
	}
}

func TestNativeOwnerFreezesTheBaselineBeforeGuestEffects(t *testing.T) {
	probe := newProbe()
	probe.Touch()
	probe.noteReady()
	baseline, err := probe.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	probe.Touch()
	probe.noteReady()
	encoded, err := probe.encodeReport(`{}`, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var got report
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatal(err)
	}
	if got.Baseline == nil || *got.Baseline != baseline || got.Effects.Calls != 2 || got.Effects.Ready != 2 {
		t.Fatalf("baseline advanced with guest effects: %s", encoded)
	}
	if _, err := probe.Begin(t.Context()); err == nil {
		t.Fatal("a second caller replaced the baseline")
	}
}

func TestNativeProbeDoesNotBeginBeforeReadiness(t *testing.T) {
	probe := newProbe()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	probe.noteReady()
	if _, err := probe.Begin(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("begin: %v", err)
	}
	encoded, err := probe.encodeReport(`{}`, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var got report
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatal(err)
	}
	var failure struct{ Error string }
	if err := json.Unmarshal(got.Carrier, &failure); err != nil {
		t.Fatal(err)
	}
	if got.Baseline != nil || failure.Error == "" {
		t.Fatalf("missing readiness became apparent success: %s", encoded)
	}
}

func TestInvalidNativeResultPublishesExplicitFailure(t *testing.T) {
	probe := newProbe()
	probe.noteReady()
	if _, err := probe.Begin(t.Context()); err != nil {
		t.Fatal(err)
	}
	encoded, err := probe.encodeReport("invalid JSON", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var got report
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatal(err)
	}
	var failure struct{ Error string }
	if err := json.Unmarshal(got.Carrier, &failure); err != nil {
		t.Fatal(err)
	}
	if got.Baseline == nil || failure.Error == "" {
		t.Fatalf("invalid result became apparent success: %s", encoded)
	}
}

func TestNativeBoundaryFailurePreservesItsCauseWithoutInventingReadiness(t *testing.T) {
	probe := newProbe()
	encoded, err := probe.encodeReport("", errors.New("native navigation failed"), nil)
	if err != nil {
		t.Fatal(err)
	}
	var got report
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatal(err)
	}
	var failure struct{ Error string }
	if err := json.Unmarshal(got.Carrier, &failure); err != nil {
		t.Fatal(err)
	}
	if got.Baseline != nil || failure.Error != "native navigation failed" {
		t.Fatalf("native failure lost its cause or invented a baseline: %s", encoded)
	}
}
