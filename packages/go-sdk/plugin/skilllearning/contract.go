// Package skilllearning defines the installation-owned Skill learning and review workflow.
package skilllearning

import (
	"errors"
	"github.com/Tangerg/flame/packages/go-sdk/skill"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/identity"
	qimumcp "github.com/Tangerg/qimu/mcp"
	"github.com/Tangerg/qimu/portable"
)

const (
	MaxTranscriptBytes         = 2 << 20
	MaxModelInputBytes         = 512 << 10
	MaxModelOutputTokens int64 = 4096
	MaxModelCalls              = 2
	MaxCallbackBytes     int64 = 16 << 20
)

type Run struct {
	Library   string `json:"library"`
	SessionID string `json:"sessionId"`
	ToolCalls int64  `json:"toolCalls" jsonschema:"minimum=0"`
}

func (r Run) Validate() error {
	if r.Library == "" {
		return errors.New("skill learning: an admitted library is required")
	}
	if err := identity.ValidateSession(r.SessionID); err != nil {
		return err
	}
	if r.ToolCalls < 0 {
		return errors.New("skill learning: tool call count cannot be negative")
	}
	return nil
}

type Skill struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	Instructions string `json:"instructions"`
}

type Proposal struct {
	Previous string `json:"previous,omitzero"`
	Skill    Skill  `json:"skill"`
	Revises  bool   `json:"revises"`
}

type MineResult struct {
	Proposal *Proposal `json:"proposal,omitzero"`
}

type ArchiveRequest struct {
	Now time.Time `json:"now"`
}

type ArchiveAdvice struct {
	InactiveSince *time.Time `json:"inactiveSince,omitzero"`
}

func (a ArchiveAdvice) Validate(observedAt time.Time) error {
	if observedAt.IsZero() {
		return errors.New("skill learning: archive observation time is required")
	}
	if a.InactiveSince != nil && (a.InactiveSince.IsZero() || !a.InactiveSince.Before(observedAt)) {
		return errors.New("skill learning: archive cutoff must precede the captured observation")
	}
	return nil
}

type Conversation struct {
	MessageCount int64  `json:"messageCount" jsonschema:"minimum=0"`
	Transcript   string `json:"transcript"`
}

type ReadSkill struct {
	Name string `json:"name"`
}

type ReadResult struct {
	Revision string `json:"revision,omitzero"`
	Skill    *Skill `json:"skill,omitzero"`
}

func MineMethod() (*qimumcp.Method[Run, MineResult], error) {
	return qimumcp.NewMethod[Run, MineResult](qimumcp.MethodConfig{Name: "flame.skill-learning/mine.v2", Kind: portable.Mutation, Target: extension.ScopeInstallation, Description: "Mine a completed Run using bounded collaborators. Commit an admitted proposal and its original mining receipt in the same transaction."})
}

func ArchiveMethod() (*qimumcp.Method[ArchiveRequest, ArchiveAdvice], error) {
	return qimumcp.NewMethod[ArchiveRequest, ArchiveAdvice](qimumcp.MethodConfig{Name: "flame.skill-learning/archive.v2", Kind: portable.Mutation, Target: extension.ScopeInstallation, Description: "Return an optional absolute activity cutoff for the supplied observation time. The caller alone admits and commits archival."})
}

func ConversationMethod() (*qimumcp.Method[struct{}, Conversation], error) {
	return qimumcp.NewMethod[struct{}, Conversation](qimumcp.MethodConfig{Name: "flame.skill-learning/conversation.v1", Kind: portable.Query, Target: extension.ScopeCall, Description: "Read the committed conversation captured for this maintenance call."})
}

func ReadSkillMethod() (*qimumcp.Method[ReadSkill, ReadResult], error) {
	return qimumcp.NewMethod[ReadSkill, ReadResult](qimumcp.MethodConfig{Name: "flame.skill-learning/read.v1", Kind: portable.Query, Target: extension.ScopeCall, Description: "Read one admitted current user skill before proposing its revision. Absent or invalid documents return no skill."})
}

type Admission struct {
	Accepted bool `json:"accepted"`
}

func AdmitMethod() (*qimumcp.Method[Proposal, Admission], error) {
	return qimumcp.NewMethod[Proposal, Admission](qimumcp.MethodConfig{Name: "flame.skill-learning/admit.v1", Kind: portable.Query, Target: extension.ScopeCall, Description: "Validate a mined proposal against the source and user library admitted for this mining call."})
}
func (p Proposal) Draft(run Run) Draft {
	return Draft{Library: run.Library, Scope: skill.User, Skill: p.Skill, Origin: skill.Mined, SourceSession: run.SessionID, Revises: p.Revises, Previous: p.Previous}
}
func Contract() (*portable.Capability, error) {
	mine, err := MineMethod()
	if err != nil {
		return nil, err
	}
	archive, err := ArchiveMethod()
	if err != nil {
		return nil, err
	}
	propose, e1 := ProposeMethod()
	queue, e2 := QueueMethod()
	approve, e3 := ApproveMethod()
	confirm, e4 := ConfirmMethod()
	reject, e5 := RejectMethod()
	discover, e6 := DiscoverMethod()
	capture, e7 := CaptureMethod()
	if err := errors.Join(e1, e2, e3, e4, e5, e6, e7); err != nil {
		return nil, err
	}
	return portable.NewCapability(portable.CapabilityConfig{Reference: Reference, Composition: portable.Selection, Scope: extension.ScopeInstallation}, mine.Descriptor(), archive.Descriptor(), propose.Descriptor(), queue.Descriptor(), approve.Descriptor(), confirm.Descriptor(), reject.Descriptor(), discover.Descriptor(), capture.Descriptor())
}
