package modelref

import (
	json "encoding/json/v2"
	"errors"
	"reflect"
	"slices"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/packages/go-sdk/instant"
	flamemodel "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"

	framecall "github.com/Tangerg/frame/call"
	framevalue "github.com/Tangerg/frame/value"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/embedding"
)

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
		return errors.New("modelref: unknown endpoint kind")
	}
}

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
		return errors.New("modelref: chat endpoint requires a chat request")
	}
	return r.Chat.Validate()
}

func (r EmbeddingEndpointRequest) validate() error {
	if r.Embedding == nil {
		return errors.New("modelref: embedding endpoint requires an embedding request")
	}
	return r.Embedding.Validate()
}

func (r ChatEndpointRequest) clone() EndpointRequest {
	return ChatEndpointRequest{Chat: r.Chat.Clone()}
}

func (r EmbeddingEndpointRequest) clone() EndpointRequest {
	return EmbeddingEndpointRequest{Embedding: cloneEmbeddingRequest(r.Embedding)}
}

type EndpointResponse interface {
	Kind() EndpointKind
	validate() error
	reportedUsage() *chat.Usage
}

type ChatEndpointResponse struct{ Chat *chat.Response }

var errNullEndpointResponse = errors.New("modelref: null endpoint response")

type EmbeddingEndpointResponse struct{ Embedding *embedding.Response }

func (ChatEndpointResponse) Kind() EndpointKind      { return EndpointChat }
func (EmbeddingEndpointResponse) Kind() EndpointKind { return EndpointEmbedding }

func (r ChatEndpointResponse) validate() error {
	if r.Chat == nil {
		return errNullEndpointResponse
	}
	return r.Chat.Validate()
}

func (r EmbeddingEndpointResponse) validate() error {
	if r.Embedding == nil {
		return errNullEndpointResponse
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

type EndpointInvocation struct {
	ID        string
	Caller    framecall.Invocation
	Provider  string
	Model     string
	SessionID identity.SessionID
	Request   EndpointRequest
	StartedAt time.Time
}

func (i EndpointInvocation) Kind() EndpointKind { return i.Request.Kind() }

const MaximumInvocationIDCharacters = 256

const endpointIDPrefix = "endpoint_"

func NewEndpointID(entropy string) string { return endpointIDPrefix + entropy }

func ValidateInvocationID(value string) error {
	return identity.ValidateURISafeASCII(value, MaximumInvocationIDCharacters)
}

func (i EndpointInvocation) Validate() error {
	if err := ValidateInvocationID(i.ID); err != nil {
		return err
	}
	if err := extension.ValidateInvocation(i.Caller); err != nil {
		return err
	}
	if i.Caller.Target.Scope != extension.ScopeCall || i.Caller.Target.Reference != i.Caller.ID || i.Caller.ID == i.ID || i.StartedAt.IsZero() {
		return errors.New("modelref: endpoint invocation requires a distinct calling model operation")
	}
	if _, err := flamemodel.New(i.Provider, i.Model); err != nil {
		return err
	}
	switch i.Request.(type) {
	case nil:
		return errors.New("modelref: endpoint invocation requires a request")
	case ChatEndpointRequest, EmbeddingEndpointRequest:
		return i.Request.validate()
	default:
		return errors.New("modelref: endpoint request must be a value variant")
	}
}

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
	return EndpointEvidenceRule{}, errors.New("modelref: unknown endpoint state")
}

func (s InvocationState) Validate() error   { _, err := s.rule(); return err }
func (s InvocationState) HasResponse() bool { rule, _ := s.rule(); return rule.Response }
func (s InvocationState) Settled() bool     { rule, _ := s.rule(); return rule.Settled }

type EndpointOutcome struct {
	Cost       accounting.Cost
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
		switch o.Response.(type) {
		case ChatEndpointResponse, EmbeddingEndpointResponse:
		default:
			return "", errors.New("modelref: endpoint response must be a value variant")
		}
		if o.Response.Kind() != invocation.Kind() {
			return "", errors.New("modelref: endpoint response belongs to another request kind")
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

func UsageMatchesResponse(usage *chat.Usage, response *chat.Response) bool {
	var reported *chat.Usage
	if response != nil && response.Metadata != nil {
		reported = response.Metadata.Usage
	}
	return reflect.DeepEqual(usage, reported)
}

func embeddingReportedUsage(response *embedding.Response) *chat.Usage {
	if response == nil || response.Metadata == nil || response.Metadata.Usage == nil {
		return nil
	}
	return &chat.Usage{InputTokens: response.Metadata.Usage.InputTokens}
}

type EndpointRecord struct {
	Cost           accounting.Cost
	Usage          *chat.Usage
	RunID          identity.RunID
	SegmentID      identity.SegmentID
	Invocation     EndpointInvocation
	State          InvocationState
	Failure        string
	FinishedAt     time.Time
	InvocationJSON framevalue.JSON
	RequestJSON    framevalue.JSON
	ResponseJSON   framevalue.JSON
}

type EndpointObservation struct {
	record      EndpointRecord
	response    *chat.Response
	servedModel string
}

func (o EndpointObservation) withKnownResponse() (EndpointObservation, *chat.Usage, error) {
	raw := o.record.ResponseJSON.Bytes()
	switch o.record.Invocation.Kind() {
	case EndpointChat:
		response, err := decodeEndpointResponse[chat.Response](raw)
		if err != nil {
			return EndpointObservation{}, nil, err
		}
		o.response = response
		if response.Metadata == nil {
			return o, nil, nil
		}
		if response.Metadata.Model != "" {
			o.servedModel = response.Metadata.Model
		}
		return o, response.Metadata.Usage, nil
	case EndpointEmbedding:
		response, err := decodeEndpointResponse[embedding.Response](raw)
		if err != nil {
			return EndpointObservation{}, nil, err
		}
		if response.Metadata != nil && response.Metadata.Model != "" {
			o.servedModel = response.Metadata.Model
		}
		return o, embeddingReportedUsage(response), nil
	default:
		return o, nil, nil
	}
}

func decodeEndpointResponse[R any, P interface {
	*R
	Validate() error
}](raw []byte) (*R, error) {
	var response *R
	if err := json.Unmarshal(raw, &response, json.RejectUnknownMembers(true)); err != nil {
		return nil, err
	}
	if response == nil {
		return nil, errNullEndpointResponse
	}
	return response, P(response).Validate()
}

func RestoreEndpointObservation(record EndpointRecord) (EndpointObservation, error) {
	var zero EndpointObservation
	if err := record.Invocation.Validate(); err != nil {
		return zero, err
	}
	if (record.RunID.IsZero()) != record.SegmentID.IsZero() {
		return zero, errors.New("modelref: endpoint execution association is incomplete")
	}
	if !record.RunID.IsZero() {
		if record.Invocation.SessionID.IsZero() {
			return zero, errors.New("modelref: endpoint run has no session")
		}
		if err := record.RunID.Validate(); err != nil {
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
		var err error
		observation, reported, err = observation.withKnownResponse()
		if err != nil {
			return zero, err
		}
	}
	if !reflect.DeepEqual(record.Usage, reported) {
		return zero, errors.New("modelref: endpoint usage differs from known response")
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
		return errors.New("modelref: endpoint admission time is required")
	}
	if err := instant.Validate(startedAt); err != nil {
		return err
	}
	if !rule.Settled {
		if !finishedAt.IsZero() || response != rule.Response || (failure != "") != rule.Failure {
			return errors.New("modelref: started endpoint carries completion")
		}
		return nil
	}
	if finishedAt.IsZero() || finishedAt.Before(startedAt) {
		return errors.New("modelref: endpoint completion time is invalid")
	}
	if err := instant.Validate(finishedAt); err != nil {
		return err
	}
	if rule.Response != response {
		return errors.New("modelref: endpoint response differs from its state")
	}
	if rule.Failure != (failure != "") {
		return errors.New("modelref: endpoint failure differs from its state")
	}
	return nil
}

func validateEndpointPrice(cost accounting.Cost, usage *chat.Usage) error {
	if _, priced := cost.USD(); priced && usage == nil {
		return errors.New("modelref: endpoint price has no reported usage")
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
