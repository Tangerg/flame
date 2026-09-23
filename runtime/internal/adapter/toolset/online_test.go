package toolset

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/scope/tools/web"
	"github.com/Tangerg/scope/tools/web/jina"

	"github.com/Tangerg/flame/runtime/internal/httporigin"
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

func TestOnlineHTTPClientBlocksCredentialRedirectAcrossOrigins(t *testing.T) {
	var targetHit atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		targetHit.Store(true)
	}))
	t.Cleanup(target.Close)
	source := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("source request did not carry the provider credential")
		}
		http.Redirect(writer, request, target.URL, http.StatusTemporaryRedirect)
	}))
	t.Cleanup(source.Close)

	client, err := jina.NewClient(jina.Config{
		APIKey: "test-key", FetchBaseURL: source.URL, HTTPClient: newOnlineHTTPClient(),
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Fetch(t.Context(), &web.FetchRequest{URL: "https://example.test"})
	if !errors.Is(err, httporigin.ErrCrossOriginRedirect) {
		t.Fatalf("Fetch error = %v, want ErrCrossOriginRedirect", err)
	}
	if targetHit.Load() {
		t.Fatal("cross-origin online-provider redirect reached the target")
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

// TestOnlineReadToolsSettleTheirOwnFailures records which network tools may end
// a Run tree. A fetch and a search read, so an upstream outage is that call's
// outcome and the model can react to it; http_request carries the model's own
// method, so a failed POST or DELETE may have left an effect this process
// cannot prove and stays unclassified.
func TestOnlineReadToolsSettleTheirOwnFailures(t *testing.T) {
	built, err := buildOnline(OnlineConfig{
		JinaAPIKey: "jina-key", TavilyAPIKey: "tavily-key", HTTPAllowedHosts: []string{"api.example.com"},
	})
	if err != nil {
		t.Fatal(err)
	}
	// Scope's own tools wrap inner tools too, so the question is specifically
	// whether this package's definite-outcome decorator is the outermost layer.
	classified := map[string]bool{}
	for _, executable := range built {
		_, definite := executable.(*callDecorator)
		classified[executable.Definition().Name] = definite
	}
	for name, want := range map[string]bool{
		"web_fetch": true, "web_search": true, "http_request": false,
	} {
		if got, present := classified[name]; !present || got != want {
			t.Errorf("%s definite-outcome classification = %t (present=%t), want %t", name, got, present, want)
		}
	}
}
