package main

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"log"
	"os"

	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	"github.com/Tangerg/frame/portable"

	"github.com/Tangerg/flame/packages/go-sdk/extension"

	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	capture "github.com/Tangerg/flame/plugins/web/internal/domain"
	"github.com/Tangerg/flame/plugins/web/internal/wire"

	"github.com/Tangerg/scope/tools/httpreq"
	"github.com/Tangerg/scope/tools/web"
)

func main() {
	contract, err := define()
	if err != nil {
		log.Fatal(err)
	}
	data, err := json.Marshal(contract.Descriptor(), json.Deterministic(true), jsontext.WithIndent("  "))
	if err != nil {
		log.Fatal(err)
	}
	if _, err := os.Stdout.Write(append(data, '\n')); err != nil {
		log.Fatal(err)
	}
}

func define() (*plugincontract.Capability, error) {
	search, err := pluginmcp.NewMethod[web.SearchRequest, wire.SearchResponse](portable.MethodConfig{Name: "flame.web/search.v1", Kind: portable.Query, Target: extension.ScopeInstallation, Description: "Search through configured Tavily credentials with a query up to 8192 bytes, honoring explicit domain and supported recency filters. Each call queries the provider; results are bounded to 128 KiB."})
	if err != nil {
		return nil, err
	}
	fetch, err := pluginmcp.NewMethod[web.FetchRequest, capture.Page](portable.MethodConfig{Name: "flame.web/fetch.v1", Kind: portable.Query, Target: extension.ScopeInstallation, Description: "Fetch a URL through configured Jina Reader credentials and capture a private immutable snapshot. Return its first 8192 Unicode codepoints and exact continuation offset."})
	if err != nil {
		return nil, err
	}
	browse, err := pluginmcp.NewMethod[wire.BrowseInput, capture.Page](portable.MethodConfig{Name: "flame.web/browse.v1", Kind: portable.Query, Target: extension.ScopeInstallation, Description: "Read a bounded page of a previously captured snapshot by ID and Unicode codepoint offset. This reads captured material without network I/O; an evicted snapshot is refused."})
	if err != nil {
		return nil, err
	}
	request, err := pluginmcp.NewMethod[httpreq.Request, wire.HTTPResponse](portable.MethodConfig{Name: "flame.web/request.v1", Kind: portable.Command, Target: extension.ScopeInstallation, Description: "Execute one HTTP request within the installed host allowlist. GET and HEAD are allowed; other methods are refused. Responses are bounded to 256 KiB. Preserve the command identity: an unconfirmed request must never be resubmitted."})
	if err != nil {
		return nil, err
	}
	return plugincontract.NewCapability(plugincontract.CapabilityConfig{Reference: "flame.web.v1", Composition: plugincontract.Collection, Scope: extension.ScopeInstallation}, search.Descriptor(), fetch.Descriptor(), browse.Descriptor(), request.Descriptor())
}
