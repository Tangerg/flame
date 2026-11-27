package researchartifact

//go:generate sh -c "go run ./cmd/descriptor > ../deliverables/contracts/research-artifact.json"
//go:generate sh -c "go run ./cmd/descriptor > ../knowledgebase/package/io.github.tangerg.flame/contracts/research-artifact.generated.json"

import (
	"github.com/Tangerg/flame/packages/go-sdk/extension"
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	"github.com/Tangerg/frame"
)

const Reference = "flame.research-artifact.v1"
const Read = "flame.research-artifact/read.v1"

type Request struct {
	Project  string `json:"project" jsonschema:"minLength=1,maxLength=256"`
	ID       string `json:"id" jsonschema:"pattern=^[a-f0-9]{64}$"`
	Revision string `json:"revision" jsonschema:"pattern=^[a-f0-9]{64}$"`
	Object   string `json:"object,omitempty" jsonschema:"maxLength=2048"`
}

type Evidence struct {
	Available      bool   `json:"available"`
	Title          string `json:"title,omitempty"`
	Format         string `json:"format,omitempty"`
	AcceptedSHA256 string `json:"acceptedSha256,omitempty"`
	Head           string `json:"head,omitempty"`
	HeadSHA256     string `json:"headSha256,omitempty"`
	ObjectAtHead   bool   `json:"objectAtHead"`
}

func Contract() (*plugincontract.Capability, *pluginmcp.Method[Request, Evidence], error) {
	method, err := pluginmcp.NewMethod[Request, Evidence](frame.MethodConfig{
		Name: Read, Kind: frame.Query, Target: extension.ScopeInstallation,
		Description: "Check Project access, exact native revision and optional chart identity at the original revision and current head.",
	})
	if err != nil {
		return nil, nil, err
	}
	capability, err := plugincontract.NewCapability(plugincontract.CapabilityConfig{
		Reference: Reference, Scope: extension.ScopeInstallation, Composition: plugincontract.Collection,
	}, method.Descriptor())
	return capability, method, err
}
