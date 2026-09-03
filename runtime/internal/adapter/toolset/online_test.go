package toolset

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Tangerg/scope/tools/web"
	"github.com/Tangerg/scope/tools/web/jina"
)

func TestOnlineHTTPClientBoundsProviderResponseBeforeSDKDecode(t *testing.T) {
	body := `{"data":{"content":"` + strings.Repeat("x", int(maxOnlineResponseFrameBytes)) + `"}}`
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(body))
	}))
	t.Cleanup(server.Close)

	client, err := jina.NewClient(jina.Config{
		APIKey: "test-key", FetchBaseURL: server.URL, HTTPClient: newOnlineHTTPClient(),
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Fetch(t.Context(), &web.FetchRequest{URL: "https://example.test"})
	if !errors.Is(err, errOnlineResponseFrameTooLarge) {
		t.Fatalf("Fetch error = %v, want errOnlineResponseFrameTooLarge", err)
	}
}

func TestBuildOnlineRejectsMalformedAPIKeysDuringAssembly(t *testing.T) {
	tests := []struct {
		name   string
		online OnlineConfig
		label  string
		secret string
	}{
		{name: "blank Jina key", online: OnlineConfig{JinaAPIKey: " \t"}, label: "web fetch (jina)", secret: " \t"},
		{name: "Jina key with surrounding whitespace", online: OnlineConfig{JinaAPIKey: " secret-jina "}, label: "web fetch (jina)", secret: "secret-jina"},
		{name: "Tavily key with newline", online: OnlineConfig{TavilyAPIKey: "secret-tavily\n"}, label: "web search (tavily)", secret: "secret-tavily"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := buildOnline(test.online)
			if err == nil {
				t.Fatal("buildOnline error = nil, want malformed credential error")
			}
			if !strings.Contains(err.Error(), test.label) {
				t.Fatalf("buildOnline error = %q, want tool label %q", err, test.label)
			}
			if strings.TrimSpace(test.secret) != "" && strings.Contains(err.Error(), strings.TrimSpace(test.secret)) {
				t.Fatalf("buildOnline error leaked credential material: %q", err)
			}
		})
	}
}
