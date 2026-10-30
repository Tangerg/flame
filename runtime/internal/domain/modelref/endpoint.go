package modelref

import (
	json "encoding/json/v2"
	"errors"
	"github.com/Tangerg/flame/qimu/portable"
	"reflect"
	"slices"
	"time"

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

// EndpointInvocation is admission of one real provider request, distinct from
// its logical calling operation. Its immutable input is persisted before I/O.
// Kind selects which request payload is present; the other is nil.
type EndpointInvocation struct {
	ID        string
	Kind      EndpointKind
	Caller    portable.Invocation
	Provider  string
	Model     string
	SessionID string
	Request   *chat.Request
	Embedding *embedding.Request
	StartedAt time.Time
}

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
	switch i.Kind {
	case EndpointChat:
		if i.Request == nil || i.Embedding != nil {
			return errors.New("models: chat endpoint requires exactly a chat request")
		}
		return i.Request.Validate()
	case EndpointEmbedding:
		if i.Embedding == nil || i.Request != nil {
			return errors.New("models: embedding endpoint requires exactly an embedding request")
		}
		return i.Embedding.Validate()
	default:
		return errors.New("models: endpoint invocation has an unknown kind")
	}
}

type EndpointState string

const (
	EndpointStarted   EndpointState = "started"
	EndpointCompleted EndpointState = "completed"
	EndpointFailed    EndpointState = "failed"
	EndpointUnknown   EndpointState = "unknown"
)

type EndpointEvidenceRule struct {
	State    EndpointState
	Response bool
	Failure  bool
	Settled  bool
}

func EndpointEvidenceRules() []EndpointEvidenceRule {
	return []EndpointEvidenceRule{
		{State: EndpointStarted},
		{State: EndpointCompleted, Response: true, Settled: true},
		{State: EndpointFailed, Response: true, Failure: true, Settled: true},
		{State: EndpointUnknown, Failure: true, Settled: true},
	}
}

func (EndpointState) Values() []EndpointState {
	rules := EndpointEvidenceRules()
	values := make([]EndpointState, len(rules))
	for i, rule := range rules {
		values[i] = rule.State
	}
	return values
}

func (s EndpointState) rule() (EndpointEvidenceRule, error) {
	for _, rule := range EndpointEvidenceRules() {
		if rule.State == s {
			return rule, nil
		}
	}
	return EndpointEvidenceRule{}, errors.New("models: unknown endpoint state")
}

func (s EndpointState) Validate() error   { _, err := s.rule(); return err }
func (s EndpointState) HasResponse() bool { rule, _ := s.rule(); return rule.Response }

// EndpointOutcome is the provider observation submitted for settlement. Admission
// derives its state; a caller cannot label missing evidence as a known failure.
type EndpointOutcome struct {
	CostUSD           *float64
	Response          *chat.Response
	EmbeddingResponse *embedding.Response
	FinishedAt        time.Time
	Failure           string
}

func (o EndpointOutcome) State(invocation EndpointInvocation) (EndpointState, error) {
	if err := invocation.Validate(); err != nil {
		return "", err
	}
	var known bool
	switch invocation.Kind {
	case EndpointChat:
		if o.EmbeddingResponse != nil {
			return "", errors.New("models: chat endpoint cannot settle an embedding response")
		}
		known = o.Response != nil
		if known {
			if err := o.Response.Validate(); err != nil {
				return "", err
			}
		}
	case EndpointEmbedding:
		if o.Response != nil {
			return "", errors.New("models: embedding endpoint cannot settle a chat response")
		}
		known = o.EmbeddingResponse != nil
		if known {
			if err := o.EmbeddingResponse.Validate(); err != nil {
				return "", err
			}
		}
	}
	state := EndpointUnknown
	if known {
		state = EndpointCompleted
		if o.Failure != "" {
			state = EndpointFailed
		}
	}
	if err := validateEndpointEvidence(state, known, o.Failure, invocation.StartedAt, o.FinishedAt); err != nil {
		return "", err
	}
	if err := validateEndpointPrice(o.CostUSD, o.ReportedUsage()); err != nil {
		return "", err
	}
	return state, nil
}

func (o EndpointOutcome) ReportedUsage() *chat.Usage {
	if o.Response != nil && o.Response.Metadata != nil {
		return cloneEndpointUsage(o.Response.Metadata.Usage)
	}
	return embeddingReportedUsage(o.EmbeddingResponse)
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
	CostUSD        *float64
	Usage          *chat.Usage
	SessionID      string
	RunID          string
	SegmentID      string
	Invocation     EndpointInvocation
	State          EndpointState
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
		switch record.Invocation.Kind {
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
	if err := validateEndpointPrice(record.CostUSD, reported); err != nil {
		return zero, err
	}
	return observation, nil
}

func validateEndpointEvidence(state EndpointState, response bool, failure string, startedAt, finishedAt time.Time) error {
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

func validateEndpointPrice(cost *float64, usage *chat.Usage) error {
	if _, err := accounting.CostFromOptional(cost); err != nil {
		return err
	}
	if cost != nil && usage == nil {
		return errors.New("models: endpoint price has no reported usage")
	}
	return nil
}

func (o EndpointObservation) Validate() error          { return o.record.State.Validate() }
func (o EndpointObservation) State() EndpointState     { return o.record.State }
func (o EndpointObservation) HasResponse() bool        { return o.record.State.HasResponse() }
func (o EndpointObservation) Open() bool               { return o.record.State == EndpointStarted }
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
		record.Invocation.Request = record.Invocation.Request.Clone()
	}
	if record.Invocation.Embedding != nil {
		record.Invocation.Embedding = cloneEmbeddingRequest(record.Invocation.Embedding)
	}
	if record.CostUSD != nil {
		record.CostUSD = new(*record.CostUSD)
	}
	record.Usage = cloneEndpointUsage(record.Usage)
	return record
}

func cloneEmbeddingRequest(r *embedding.Request) *embedding.Request {
	return &embedding.Request{Texts: slices.Clone(r.Texts), Options: r.Options.Clone()}
}
