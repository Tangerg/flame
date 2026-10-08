package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
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
