package runs

import (
	"fmt"
	"slices"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

// ResumeAcceptance carries the decided answer transitions. The same Item
// values supply executor input and the opening's transcript compare-and-swap.
type ResumeAcceptance struct {
	sessionID       string
	expected        Pending
	itemChanges     []transcript.Replacement
	rememberedRules []approval.Rule
}

func prepareResumeAcceptance(
	sessionID string,
	expected Pending,
	items map[string]transcript.Item,
	responses []ResumeResponse,
) (ResumeAcceptance, error) {
	if err := identity.ValidateSession(sessionID); err != nil {
		return ResumeAcceptance{}, fmt.Errorf("runs: resume acceptance: %w", err)
	}
	interrupts, err := expected.ProjectInterrupts(items)
	if err != nil {
		return ResumeAcceptance{}, fmt.Errorf("runs: project open interrupts: %w", err)
	}
	changes, err := settleResumeAnswers(interrupts, items, responses)
	if err != nil {
		return ResumeAcceptance{}, err
	}
	return ResumeAcceptance{sessionID: sessionID, expected: expected.Clone(), itemChanges: changes}, nil
}

func (r ResumeAcceptance) SessionID() string { return r.sessionID }

func (r ResumeAcceptance) Pending() Pending { return r.expected.Clone() }

func (r ResumeAcceptance) RememberedRules() []approval.Rule { return slices.Clone(r.rememberedRules) }

func (r ResumeAcceptance) ItemReplacements() []transcript.Replacement {
	return slices.Clone(r.itemChanges)
}
