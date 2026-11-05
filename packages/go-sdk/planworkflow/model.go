package planworkflow

import (
	json "encoding/json/v2"
	"errors"
	"fmt"
	"strings"

	framevalue "github.com/Tangerg/frame/value"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
)

const Reference = "flame.plan-workflow.v1"

type Step struct {
	Description string     `json:"description"`
	Status      PlanStatus `json:"status"`
}

func (s Step) Validate() error {
	if strings.TrimSpace(s.Description) == "" {
		return errors.New("plan: step description is required")
	}
	switch s.Status {
	case PlanStatusPending, PlanStatusInProgress, PlanStatusCompleted:
		return nil
	default:
		return errors.New("plan: unknown step status")
	}
}

type AuthoredDocument struct {
	Document
	Source extension.ContributionSource
}

type Proposal struct {
	Steps []Step `json:"steps"`
}

// A digest identifies the immutable checklist across Sessions and Run boundaries.
// Local revision numbers order the references made by one Session.
type Document struct {
	Proposal
	Digest framevalue.Digest `json:"digest"`
}

func Seal(proposal Proposal) (Document, error) {
	if proposal.Steps == nil {
		proposal.Steps = []Step{}
	}
	body, err := json.Marshal(proposal.Steps, json.Deterministic(true))
	if err != nil {
		return Document{}, err
	}
	return Document{Proposal: proposal, Digest: framevalue.Hash(body)}, nil
}

func (d Document) Verify() error {
	sealed, err := Seal(d.Proposal)
	if err != nil {
		return err
	}
	if sealed.Digest != d.Digest {
		return errors.New("plan: document content differs from its immutable reference")
	}
	return nil
}

// A refusal is an authored decision. A lost transport response has no such evidence.
type Decision struct {
	Document *Document `json:"document,omitzero"`
	Refusal  string    `json:"refusal,omitzero"`
}

func (d Decision) Open() (Document, error) {
	if (d.Document == nil) != (d.Refusal != "") {
		return Document{}, errors.New("plan: incomplete author decision")
	}
	if d.Document == nil {
		return Document{}, fmt.Errorf("%w: %s", ErrRefused, d.Refusal)
	}
	return *d.Document, d.Document.Verify()
}

var ErrRefused = errors.New("plan: author refused the proposal")

type PlanStatus string

const (
	PlanStatusPending    PlanStatus = "pending"
	PlanStatusInProgress PlanStatus = "in_progress"
	PlanStatusCompleted  PlanStatus = "completed"
)
