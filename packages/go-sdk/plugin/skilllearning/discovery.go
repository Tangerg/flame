package skilllearning

import (
	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/skill"
	qimumcp "github.com/Tangerg/qimu/mcp"
	"github.com/Tangerg/qimu/portable"
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

func DiscoverMethod() (*qimumcp.Method[Candidates, Catalog], error) {
	return qimumcp.NewMethod[Candidates, Catalog](qimumcp.MethodConfig{Name: "flame.skills/discover.v1", Kind: portable.Query, Target: extension.ScopeInstallation, Description: "Select among the host-admitted Skill readers for a human catalog."})
}
func CaptureMethod() (*qimumcp.Method[Candidates, Catalog], error) {
	return qimumcp.NewMethod[Candidates, Catalog](qimumcp.MethodConfig{Name: "flame.skills/capture.v1", Kind: portable.Mutation, Target: extension.ScopeInstallation, Description: "Record an interaction's original Skill source choices for exact receipt recovery."})
}
