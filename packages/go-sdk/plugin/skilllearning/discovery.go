package skilllearning

import (
	"github.com/Tangerg/flame/packages/go-sdk/extension"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	"github.com/Tangerg/flame/packages/go-sdk/skill"
	"github.com/Tangerg/frame/portable"
)

// An opaque source identifies an admitted reader. A provider can choose a
// candidate by name and source without receiving a workspace path or file.
type Candidate struct {
	Name   string      `json:"name"`
	Source string      `json:"source"`
	Scope  skill.Scope `json:"scope"`
}
type Candidates struct {
	Library string      `json:"library"`
	Items   []Candidate `json:"items"`
}
type Choice struct {
	Name   string `json:"name"`
	Source string `json:"source"`
}
type Catalog struct {
	Entries []Choice `json:"entries"`
}

func DiscoverMethod() (*pluginmcp.Method[Candidates, Catalog], error) {
	return pluginmcp.NewMethod[Candidates, Catalog](portable.MethodConfig{Name: "flame.skills/discover.v1", Kind: portable.Query, Target: extension.ScopeInstallation, Description: "Select among the host-admitted Skill readers for a human catalog."})
}
func CaptureMethod() (*pluginmcp.Method[Candidates, Catalog], error) {
	return pluginmcp.NewMethod[Candidates, Catalog](portable.MethodConfig{Name: "flame.skills/capture.v1", Kind: portable.Command, Target: extension.ScopeInstallation, Description: "Record an interaction's original Skill source choices for exact receipt recovery."})
}
