package modelref

import (
	json "encoding/json/v2"
	"errors"
	"reflect"
	"slices"
	"time"

	"github.com/Tangerg/flame/qimu/portable"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/packages/go-sdk/instant"
	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/qimu/jsonvalue"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/embedding"
)

// EndpointKind names the provider operation a physical endpoint records. Chat
// and embedding share one admission/evidence/accounting owner; each kind carries
// exactly its own request and response payload.
type EndpointKind string

const (
	EndpointChat      EndpointKind = "chat"
	EndpointEmbedding EndpointKind = "embedding"
)

func (k EndpointKind) Validate() error {
	switch k {
	case EndpointChat, EndpointEmbedding:
		return nil
	default:
		return errors.New("models: unknown endpoint kind")
	}
}

// EndpointRequest is the closed set of provider requests an endpoint admits.
// The variant is the endpoint's kind, so a kind cannot disagree with its payload.
type EndpointRequest interface {
	Kind() EndpointKind
	validate() error
	clone() EndpointRequest
}

type ChatEndpointRequest struct{ Chat *chat.Request }

type EmbeddingEndpointRequest struct{ Embedding *embedding.Request }

func (ChatEndpointRequest) Kind() EndpointKind      { return EndpointChat }
func (EmbeddingEndpointRequest) Kind() EndpointKind { return EndpointEmbedding }

func (r ChatEndpointRequest) validate() error {
	if r.Chat == nil {
		return errors.New("models: chat endpoint requires a chat request")
	}
	return r.Chat.Validate()
}

func (r EmbeddingEndpointRequest) validate() error {
	if r.Embedding == nil {
		return errors.New("models: embedding endpoint requires an embedding request")
	}
	return r.Embedding.Validate()
}

func (r ChatEndpointRequest) clone() EndpointRequest {
	return ChatEndpointRequest{Chat: r.Chat.Clone()}
}

func (r EmbeddingEndpointRequest) clone() EndpointRequest {
	return EmbeddingEndpointRequest{Embedding: cloneEmbeddingRequest(r.Embedding)}
}

// EndpointResponse is the closed set of provider responses an endpoint can
// settle with; it must be the variant of its invocation's request.
type EndpointResponse interface {
	Kind() EndpointKind
	validate() error
	reportedUsage() *chat.Usage
}

type ChatEndpointResponse struct{ Chat *chat.Response }

type EmbeddingEndpointResponse struct{ Embedding *embedding.Response }

func (ChatEndpointResponse) Kind() EndpointKind      { return EndpointChat }
func (EmbeddingEndpointResponse) Kind() EndpointKind { return EndpointEmbedding }

func (r ChatEndpointResponse) validate() error {
	if r.Chat == nil {
		return errors.New("models: null endpoint response")
	}
	return r.Chat.Validate()
}

func (r EmbeddingEndpointResponse) validate() error {
	if r.Embedding == nil {
		return errors.New("models: null endpoint response")
	}
	return r.Embedding.Validate()
}

func (r ChatEndpointResponse) reportedUsage() *chat.Usage {
	if r.Chat == nil || r.Chat.Metadata == nil {
		return nil
	}
	return cloneEndpointUsage(r.Chat.Metadata.Usage)
}

func (r EmbeddingEndpointResponse) reportedUsage() *chat.Usage {
	return embeddingReportedUsage(r.Embedding)
}

// EndpointInvocation is admission of one real provider request, distinct from
// its logical calling operation. Its immutable input is persisted before I/O.
type EndpointInvocation struct {
	ID        string
	Caller    portable.Invocation
	Provider  string
	Model     string
	SessionID string
	Request   EndpointRequest
	StartedAt time.Time
}

func (i EndpointInvocation) Kind() EndpointKind { return i.Request.Kind() }

const maxInvocationIdentityBytes = 256

// ValidateInvocationID admits exact physical or parent model-call identity.
func ValidateInvocationID(value string) error {
	return identity.ValidateURISafeASCII(value, maxInvocationIdentityBytes)
}

func (i EndpointInvocation) Validate() error {
	if err := ValidateInvocationID(i.ID); err != nil {
		return err
	}
	if err := extension.ValidateInvocation(i.Caller); err != nil {
		return err
	}
	if i.Caller.Target.Scope != extension.ScopeCall || i.Caller.Target.Reference != i.Caller.ID || i.Caller.ID == i.ID || i.StartedAt.IsZero() {
		return errors.New("models: endpoint invocation requires a distinct calling model operation")
	}
	if _, err := modelchoice.New(i.Provider, i.Model); err != nil {
		return err
	}
	if i.SessionID != "" {
		if err := identity.ValidateSession(i.SessionID); err != nil {
			return err
		}
	}
	if i.Request == nil {
		return errors.New("models: endpoint invocation requires a request")
	}
	return i.Request.validate()
}

// InvocationState is the durable observation of one model invocation, whether
// a Run's logical call or one physical endpoint attempt. It is deliberately
// smaller than a response: it distinguishes a call that never crossed the
// provider boundary from one whose final projection became indeterminate.
type InvocationState string

const (
	InvocationStarted   InvocationState = "started"
	InvocationCompleted InvocationState = "completed"
	InvocationFailed    InvocationState = "failed"
	InvocationUnknown   InvocationState = "unknown"
)

type EndpointEvidenceRule struct {
	State    InvocationState
	Response bool
	Failure  bool
	Settled  bool
}

func EndpointEvidenceRules() []EndpointEvidenceRule {
	return []EndpointEvidenceRule{
		{State: InvocationStarted},
		{State: InvocationCompleted, Response: true, Settled: true},
		{State: InvocationFailed, Response: true, Failure: true, Settled: true},
		{State: InvocationUnknown, Failure: true, Settled: true},
	}
}

func (InvocationState) Values() []InvocationState {
	rules := EndpointEvidenceRules()
	values := make([]InvocationState, len(rules))
	for i, rule := range rules {
		values[i] = rule.State
	}
	return values
}

func (s InvocationState) rule() (EndpointEvidenceRule, error) {
	for _, rule := range EndpointEvidenceRules() {
		if rule.State == s {
			return rule, nil
		}
	}
	return EndpointEvidenceRule{}, errors.New("models: unknown endpoint state")
}

func (s InvocationState) Validate() error   { _, err := s.rule(); return err }
func (s InvocationState) HasResponse() bool { rule, _ := s.rule(); return rule.Response }

// EndpointOutcome is the provider observation submitted for settlement. Admission
// derives its state; a caller cannot label missing evidence as a known failure.
type EndpointOutcome struct {
	Cost accounting.Cost
	// Response is nil when the provider's answer was never known.
	Response   EndpointResponse
	FinishedAt time.Time
	Failure    string
}

func (o EndpointOutcome) State(invocation EndpointInvocation) (InvocationState, error) {
	if err := invocation.Validate(); err != nil {
		return "", err
	}
	known := o.Response != nil
	if known {
		if o.Response.Kind() != invocation.Kind() {
			return "", errors.New("models: endpoint response belongs to another request kind")
		}
		if err := o.Response.validate(); err != nil {
			return "", err
		}
	}
	state := InvocationUnknown
	if known {
		state = InvocationCompleted
		if o.Failure != "" {
			state = InvocationFailed
		}
	}
	if err := validateEndpointEvidence(state, known, o.Failure, invocation.StartedAt, o.FinishedAt); err != nil {
		return "", err
	}
	if err := validateEndpointPrice(o.Cost, o.ReportedUsage()); err != nil {
		return "", err
	}
	return state, nil
}

func (o EndpointOutcome) ReportedUsage() *chat.Usage {
	if o.Response == nil {
		return nil
	}
	return o.Response.reportedUsage()
}

func embeddingReportedUsage(response *embedding.Response) *chat.Usage {
	if response == nil || response.Metadata == nil || response.Metadata.Usage == nil {
		return nil
	}
	return &chat.Usage{InputTokens: response.Metadata.Usage.InputTokens}
}

// EndpointRecord encodes an observation. Request and response JSON retain their
// original numeric lexemes; normalized response and model facts are derived only
// after this record is admitted by RestoreEndpointObservation.
type EndpointRecord struct {
	Cost           accounting.Cost
	Usage          *chat.Usage
	SessionID      string
	RunID          string
	SegmentID      string
	Invocation     EndpointInvocation
	State          InvocationState
	Failure        string
	FinishedAt     time.Time
	InvocationJSON jsonvalue.Value
	RequestJSON    jsonvalue.Value
	ResponseJSON   jsonvalue.Value
}

// EndpointObservation owns one admitted, immutable physical request observation.
type EndpointObservation struct {
	record      EndpointRecord
	response    *chat.Response
	servedModel string
}

func RestoreEndpointObservation(record EndpointRecord) (EndpointObservation, error) {
	var zero EndpointObservation
	if err := record.Invocation.Validate(); err != nil {
		return zero, err
	}
	if record.Invocation.SessionID != "" && record.Invocation.SessionID != record.SessionID {
		return zero, errors.New("models: endpoint session differs from its admission")
	}
	if record.SessionID != "" {
		if err := identity.ValidateSession(record.SessionID); err != nil {
			return zero, err
		}
	}
	if (record.RunID == "") != (record.SegmentID == "") {
		return zero, errors.New("models: endpoint execution association is incomplete")
	}
	if record.RunID != "" {
		if record.SessionID == "" {
			return zero, errors.New("models: endpoint run has no session")
		}
		if err := identity.ValidateRun(record.RunID); err != nil {
			return zero, err
		}
		if err := identity.ValidateSegment(record.SegmentID); err != nil {
			return zero, err
		}
	}
	if err := record.InvocationJSON.Validate(); err != nil {
		return zero, err
	}
	if err := record.RequestJSON.Validate(); err != nil {
		return zero, err
	}
	known := record.ResponseJSON.String() != ""
	if err := validateEndpointEvidence(record.State, known, record.Failure, record.Invocation.StartedAt, record.FinishedAt); err != nil {
		return zero, err
	}
	observation := EndpointObservation{record: cloneEndpointRecord(record), servedModel: record.Invocation.Model}
	var reported *chat.Usage
	if known {
		switch record.Invocation.Kind() {
		case EndpointChat:
			var response *chat.Response
			if err := json.Unmarshal(record.ResponseJSON.JSON(), &response, json.RejectUnknownMembers(true)); err != nil {
				return zero, err
			}
			if response == nil {
				return zero, errors.New("models: null endpoint response")
			}
			if err := response.Validate(); err != nil {
				return zero, err
			}
			observation.response = response
			if response.Metadata != nil {
				reported = response.Metadata.Usage
				if response.Metadata.Model != "" {
					observation.servedModel = response.Metadata.Model
				}
			}
		case EndpointEmbedding:
			var response *embedding.Response
			if err := json.Unmarshal(record.ResponseJSON.JSON(), &response, json.RejectUnknownMembers(true)); err != nil {
				return zero, err
			}
			if response == nil {
				return zero, errors.New("models: null endpoint response")
			}
			if err := response.Validate(); err != nil {
				return zero, err
			}
			reported = embeddingReportedUsage(response)
			if response.Metadata != nil && response.Metadata.Model != "" {
				observation.servedModel = response.Metadata.Model
			}
		}
	}
	if !reflect.DeepEqual(record.Usage, reported) {
		return zero, errors.New("models: endpoint usage differs from known response")
	}
	if err := validateEndpointPrice(record.Cost, reported); err != nil {
		return zero, err
	}
	return observation, nil
}

func validateEndpointEvidence(state InvocationState, response bool, failure string, startedAt, finishedAt time.Time) error {
	rule, err := state.rule()
	if err != nil {
		return err
	}
	if startedAt.IsZero() {
		return errors.New("models: endpoint admission time is required")
	}
	if err := instant.Validate(startedAt); err != nil {
		return err
	}
	if !rule.Settled {
		if !finishedAt.IsZero() || response != rule.Response || (failure != "") != rule.Failure {
			return errors.New("models: started endpoint carries completion")
		}
		return nil
	}
	if finishedAt.IsZero() || finishedAt.Before(startedAt) {
		return errors.New("models: endpoint completion time is invalid")
	}
	if err := instant.Validate(finishedAt); err != nil {
		return err
	}
	if rule.Response != response {
		return errors.New("models: endpoint response differs from its state")
	}
	if rule.Failure != (failure != "") {
		return errors.New("models: endpoint failure differs from its state")
	}
	return nil
}

func validateEndpointPrice(cost accounting.Cost, usage *chat.Usage) error {
	if _, priced := cost.USD(); priced && usage == nil {
		return errors.New("models: endpoint price has no reported usage")
	}
	return nil
}

func (o EndpointObservation) Validate() error          { return o.record.State.Validate() }
func (o EndpointObservation) State() InvocationState   { return o.record.State }
func (o EndpointObservation) HasResponse() bool        { return o.record.State.HasResponse() }
func (o EndpointObservation) Open() bool               { return o.record.State == InvocationStarted }
func (o EndpointObservation) Snapshot() EndpointRecord { return cloneEndpointRecord(o.record) }
func (o EndpointObservation) Response() *chat.Response { return o.response.Clone() }
func (o EndpointObservation) ServedModel() string      { return o.servedModel }

func cloneEndpointUsage(usage *chat.Usage) *chat.Usage {
	if usage == nil {
		return nil
	}
	value := usage.Clone()
	return &value
}

func cloneEndpointRecord(record EndpointRecord) EndpointRecord {
	if record.Invocation.Request != nil {
		record.Invocation.Request = record.Invocation.Request.clone()
	}
	record.Usage = cloneEndpointUsage(record.Usage)
	return record
}

func cloneEmbeddingRequest(r *embedding.Request) *embedding.Request {
	return &embedding.Request{Texts: slices.Clone(r.Texts), Options: r.Options.Clone()}
}

// InvocationPageAnchor is the (start time, invocation identity) position a
// newest-first page of endpoint invocations ended at; a missing anchor is the
// first page.
type InvocationPageAnchor struct {
	startedAt time.Time
	id        string
}

func NewInvocationPageAnchor(startedAt time.Time, id string) (InvocationPageAnchor, error) {
	if err := ValidateInvocationID(id); err != nil {
		return InvocationPageAnchor{}, err
	}
	return InvocationPageAnchor{startedAt: startedAt.UTC(), id: id}, nil
}

func (a InvocationPageAnchor) StartedAt() time.Time { return a.startedAt }
func (a InvocationPageAnchor) ID() string           { return a.id }
