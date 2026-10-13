package http_test

import (
	"net/http"
	"testing"
)

func TestPluginResourceURLRequiresItsLiveLeaseAndDoesNotExposeBearerCredentials(t *testing.T) {
	server := newGatedServerWithOrigins(t, nil)
	defer server.Close()
	response, err := http.Get(server.URL + "/v2/plugin-resources/unknown/io.github.tangerg.flame/components/component.js")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusGone || response.Header.Get("Cache-Control") != "no-store" || response.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("unowned resource response: %d,%v", response.StatusCode, response.Header)
	}
	request, err := http.NewRequest(http.MethodPost, server.URL+"/v2/plugin-resources/unknown/file.js", nil)
	if err != nil {
		t.Fatal(err)
	}
	post, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer post.Body.Close()
	if post.StatusCode != http.StatusUnauthorized {
		t.Fatalf("resource path bypassed bearer gate for another method: %d", post.StatusCode)
	}
}
