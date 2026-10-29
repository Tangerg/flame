// Package skilllearning defines optional learning advice. The caller retains
// proposal admission, approval, active documents, and archive commits.
package skilllearning

import (
	"errors"
	"math"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/identity"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
)

const (
	MaxTranscriptBytes         = 2 << 20
	MaxModelInputBytes         = 512 << 10
	MaxModelOutputTokens int64 = 4096
	MaxModelCalls              = 2
	MaxCallbackBytes     int64 = 16 << 20
)

type Run struct {
	SessionID string `json:"sessionId"`
	ToolCalls int64  `json:"toolCalls" jsonschema:"minimum=0"`
}

func (r Run) Validate() error {
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
	Skill   Skill `json:"skill"`
	Revises bool  `json:"revises"`
}

type MineResult struct {
	Proposal *Proposal `json:"proposal,omitzero"`
}

type ArchiveRequest struct {
	Now time.Time `json:"now"`
}

type ArchiveAdvice struct {
	AfterMillis *int64 `json:"afterMillis,omitzero" jsonschema:"minimum=1"`
}

func (a ArchiveAdvice) Duration() (time.Duration, error) {
	if a.AfterMillis == nil {
		return 0, nil
	}
	if *a.AfterMillis <= 0 || *a.AfterMillis > math.MaxInt64/int64(time.Millisecond) {
		return 0, errors.New("skill learning: invalid archive inactivity duration")
	}
	return time.Duration(*a.AfterMillis) * time.Millisecond, nil
}

type Conversation struct {
	MessageCount int64  `json:"messageCount" jsonschema:"minimum=0"`
	Transcript   string `json:"transcript"`
}

type ReadSkill struct {
	Name string `json:"name"`
}

type ReadResult struct {
	Skill *Skill `json:"skill,omitzero"`
}

func MineMethod() (*qimumcp.Method[Run, MineResult], error) {
	return qimumcp.NewMethod[Run, MineResult](qimumcp.MethodConfig{Name: "flame.skill-learning/mine.v1", Kind: portable.Mutation, Target: extension.ScopeInstallation, Description: "Apply learning cadence to a completed Run and return optional proposal advice using call-scoped history, current skill and model callbacks. Never publish or store active skills."})
}

func ArchiveMethod() (*qimumcp.Method[ArchiveRequest, ArchiveAdvice], error) {
	return qimumcp.NewMethod[ArchiveRequest, ArchiveAdvice](qimumcp.MethodConfig{Name: "flame.skill-learning/archive.v1", Kind: portable.Mutation, Target: extension.ScopeInstallation, Description: "Return an optional inactivity threshold when an archive check is due at the caller's time. The caller alone admits and commits archival."})
}

func ConversationMethod() (*qimumcp.Method[struct{}, Conversation], error) {
	return qimumcp.NewMethod[struct{}, Conversation](qimumcp.MethodConfig{Name: "flame.skill-learning/conversation.v1", Kind: portable.Query, Target: extension.ScopeCall, Description: "Read the committed conversation captured for this maintenance call."})
}

func ReadSkillMethod() (*qimumcp.Method[ReadSkill, ReadResult], error) {
	return qimumcp.NewMethod[ReadSkill, ReadResult](qimumcp.MethodConfig{Name: "flame.skill-learning/read.v1", Kind: portable.Query, Target: extension.ScopeCall, Description: "Read one admitted current user skill before proposing its revision. Absent or invalid documents return no skill."})
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
	return portable.NewCapability(portable.CapabilityConfig{Reference: "flame.skill-learning.v1", Composition: portable.Selection, Scope: extension.ScopeInstallation}, mine.Descriptor(), archive.Descriptor())
}
