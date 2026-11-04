package skilllearning

import (
	"github.com/Tangerg/flame/packages/go-sdk/extension"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	"github.com/Tangerg/flame/packages/go-sdk/skill"
	"github.com/Tangerg/frame/portable"
)

const (
	Reference   = "flame.skill-learning.v2"
	StaleReview = "stale_review"
)

// Draft binds a proposal's immutable command input to a host-admitted library
// and the active bytes against which a human can authorize publication.
type Draft struct {
	Library       string               `json:"library"`
	Scope         skill.Scope          `json:"scope"`
	Skill         Skill                `json:"skill"`
	Origin        skill.ProposalOrigin `json:"origin"`
	SourceSession string               `json:"sourceSession"`
	Revises       bool                 `json:"revises"`
	Previous      string               `json:"previous"`
}

type ReviewRef struct {
	Command string `json:"command"`
}
type Libraries struct {
	Project string `json:"project"`
	User    string `json:"user"`
}
type Queue struct {
	Proposals []ReviewRef `json:"proposals"`
}
type Permit struct {
	Decision string `json:"decision"`
}
type Publication struct {
	Proposal  ReviewRef `json:"proposal"`
	Permit    Permit    `json:"permit"`
	Published bool      `json:"published"`
}

func ProposeMethod() (*pluginmcp.Method[Draft, ReviewRef], error) {
	return pluginmcp.NewMethod[Draft, ReviewRef](portable.MethodConfig{Name: "flame.skills/propose.v1", Kind: portable.Command, Target: extension.ScopeInstallation, Description: "Stage a reviewed library draft under its immutable originating command."})
}
func QueueMethod() (*pluginmcp.Method[Libraries, Queue], error) {
	return pluginmcp.NewMethod[Libraries, Queue](portable.MethodConfig{Name: "flame.skills/reviews.v1", Kind: portable.Query, Target: extension.ScopeInstallation, Description: "List outstanding proposal commands in the admitted libraries, including publications awaiting confirmation."})
}
func ApproveMethod() (*pluginmcp.Method[ReviewRef, Permit], error) {
	return pluginmcp.NewMethod[ReviewRef, Permit](portable.MethodConfig{Name: "flame.skills/approve.v1", Kind: portable.Command, Target: extension.ScopeInstallation, Description: "Record a human decision for the current proposal and retain its original publication permit."})
}
func ConfirmMethod() (*pluginmcp.Method[Publication, struct{}], error) {
	return pluginmcp.NewMethod[Publication, struct{}](portable.MethodConfig{Name: "flame.skills/confirm.v1", Kind: portable.Command, Target: extension.ScopeInstallation, Description: "Close the original publication after the authorized filesystem owner confirms publication or a definite precondition conflict."})
}
func RejectMethod() (*pluginmcp.Method[ReviewRef, struct{}], error) {
	return pluginmcp.NewMethod[ReviewRef, struct{}](portable.MethodConfig{Name: "flame.skills/reject.v1", Kind: portable.Command, Target: extension.ScopeInstallation, Description: "Withdraw an exact pending proposal before approval."})
}
