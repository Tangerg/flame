package main

import (
	"github.com/Tangerg/flame/packages/go-sdk/extension"
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	"github.com/Tangerg/flame/plugins/web/internal/capture"
	"github.com/Tangerg/flame/plugins/web/internal/wire"

	"github.com/Tangerg/frame"
	"github.com/Tangerg/scope/tools/httpreq"
	"github.com/Tangerg/scope/tools/web"
)

func main() { plugincontract.Publish(define) }

func define() (*plugincontract.Capability, error) {
	search, err := pluginmcp.NewMethod[web.SearchRequest, wire.SearchResponse](frame.MethodConfig{Name: "flame.web/search.v1", Kind: frame.Query, Target: extension.ScopeInstallation, Description: "Search through configured Tavily credentials with a query up to 8192 bytes, honoring explicit domain and supported recency filters. Each call queries the provider; results are bounded to 128 KiB."})
	if err != nil {
		return nil, err
	}
	fetch, err := pluginmcp.NewMethod[web.FetchRequest, capture.Page](frame.MethodConfig{Name: "flame.web/fetch.v1", Kind: frame.Query, Target: extension.ScopeInstallation, Description: "Fetch a URL through configured Jina Reader credentials and capture a private immutable snapshot. Return its first 8192 Unicode codepoints and exact continuation offset."})
	if err != nil {
		return nil, err
	}
	browse, err := pluginmcp.NewMethod[wire.BrowseInput, capture.Page](frame.MethodConfig{Name: "flame.web/browse.v1", Kind: frame.Query, Target: extension.ScopeInstallation, Description: "Read a bounded page of a previously captured snapshot by ID and Unicode codepoint offset. This reads captured material without network I/O; an evicted snapshot is refused."})
	if err != nil {
		return nil, err
	}
	request, err := pluginmcp.NewMethod[httpreq.Request, wire.HTTPResponse](frame.MethodConfig{Name: "flame.web/request.v1", Kind: frame.Command, Target: extension.ScopeInstallation, Description: "Execute one HTTP request within the installed host allowlist. GET and HEAD are allowed; other methods are refused. Responses are bounded to 256 KiB. Preserve the command identity: an unconfirmed request must never be resubmitted."})
	if err != nil {
		return nil, err
	}
	return plugincontract.NewCapability(plugincontract.CapabilityConfig{Reference: "flame.web.v1", Composition: plugincontract.Collection, Scope: extension.ScopeInstallation}, search.Descriptor(), fetch.Descriptor(), browse.Descriptor(), request.Descriptor())
}
