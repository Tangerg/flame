package main

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAssetRequestsRecordEffectsEvenWhenTheResourceIsMissing(t *testing.T) {
	probe := &Probe{}
	handler := probe.assetHandler()
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
			if got := probe.Snapshot().ResourceRequests; got != tt.requests {
				t.Fatalf("resource requests = %d, want %d", got, tt.requests)
			}
		})
	}
}

func TestPeerWitnessRecordsPacketsOutsideTheFrame(t *testing.T) {
	probe := &Probe{}
	closePeer, err := probe.listenPeer()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := closePeer(); err != nil {
			t.Error(err)
		}
	})
	address := strings.TrimPrefix(probe.Snapshot().PeerEndpoint, "stun:")
	sender, err := net.Dial("udp4", address)
	if err != nil {
		t.Fatal(err)
	}
	defer sender.Close()
	if _, err := sender.Write([]byte("positive control")); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for probe.Snapshot().PeerPackets == 0 {
		if time.Now().After(deadline) {
			t.Fatal("peer witness did not observe its positive control")
		}
		time.Sleep(time.Millisecond)
	}
	if probe.Snapshot().PeerPackets != 1 {
		t.Fatalf("peer packet count = %d", probe.Snapshot().PeerPackets)
	}
}
