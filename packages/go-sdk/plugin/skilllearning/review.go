package skilllearning

import (
	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/skill"
	qimumcp "github.com/Tangerg/qimu/mcp"
	"github.com/Tangerg/qimu/portable"
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

func ProposeMethod() (*qimumcp.Method[Draft, ReviewRef], error) {
	return qimumcp.NewMethod[Draft, ReviewRef](qimumcp.MethodConfig{Name: "flame.skills/propose.v1", Kind: portable.Mutation, Target: extension.ScopeInstallation, Description: "Stage a reviewed library draft under its immutable originating command."})
}
func QueueMethod() (*qimumcp.Method[Libraries, Queue], error) {
	return qimumcp.NewMethod[Libraries, Queue](qimumcp.MethodConfig{Name: "flame.skills/reviews.v1", Kind: portable.Query, Target: extension.ScopeInstallation, Description: "List outstanding proposal commands in the admitted libraries, including publications awaiting confirmation."})
}
func ApproveMethod() (*qimumcp.Method[ReviewRef, Permit], error) {
	return qimumcp.NewMethod[ReviewRef, Permit](qimumcp.MethodConfig{Name: "flame.skills/approve.v1", Kind: portable.Mutation, Target: extension.ScopeInstallation, Description: "Record a human decision for the current proposal and retain its original publication permit."})
}
func ConfirmMethod() (*qimumcp.Method[Publication, struct{}], error) {
	return qimumcp.NewMethod[Publication, struct{}](qimumcp.MethodConfig{Name: "flame.skills/confirm.v1", Kind: portable.Mutation, Target: extension.ScopeInstallation, Description: "Close the original publication after the authorized filesystem owner confirms publication or a definite precondition conflict."})
}
func RejectMethod() (*qimumcp.Method[ReviewRef, struct{}], error) {
	return qimumcp.NewMethod[ReviewRef, struct{}](qimumcp.MethodConfig{Name: "flame.skills/reject.v1", Kind: portable.Mutation, Target: extension.ScopeInstallation, Description: "Withdraw an exact pending proposal before approval."})
}
