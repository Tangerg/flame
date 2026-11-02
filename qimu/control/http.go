package control

import (
	"encoding/json"
	"reflect"
)

// HTTPEnvelope carries one control operation. Input is decoded as that
// operation's published DTO by the authenticated host adapter.
type HTTPEnvelope struct {
	Method OperationName   `json:"method"`
	Input  json.RawMessage `json:"input"`
}

type HTTPProblem struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type OperationName string

const (
	OpValidatePlan  OperationName = "validatePlan"
	OpReady         OperationName = "ready"
	OpAcquire       OperationName = "acquire"
	OpValidateUse   OperationName = "validateUse"
	OpValidateRoute OperationName = "validateRoute"
	OpSnapshot      OperationName = "snapshot"
	OpCloseSnapshot OperationName = "closeSnapshot"
	OpPublish       OperationName = "publish"
	OpPublication   OperationName = "publication"
	OpWithdraw      OperationName = "withdraw"
	OpProve         OperationName = "prove"
	OpReconnect     OperationName = "reconnect"
	OpPoll          OperationName = "poll"
)

type operationContract struct {
	name          OperationName
	input, output reflect.Type
	mutating      bool
}

func operation[I, O any](name OperationName, mutating bool) operationContract {
	return operationContract{name, reflect.TypeFor[I](), reflect.TypeFor[O](), mutating}
}

type noResult struct{}

var operations = [...]operationContract{
	operation[LocalPlan, noResult](OpValidatePlan, false),
	operation[ReadyReport, PublicationResult](OpReady, true),
	operation[AcquireRequest, UseGrant](OpAcquire, true),
	operation[UseGrant, noResult](OpValidateUse, false),
	operation[RouteGrant, noResult](OpValidateRoute, false),
	operation[[]string, Snapshot](OpSnapshot, true),
	operation[string, noResult](OpCloseSnapshot, true),
	operation[PublicationRequest, PublicationResult](OpPublish, true),
	operation[string, PublicationResult](OpPublication, false),
	operation[Withdrawal, PublicationResult](OpWithdraw, true),
	operation[CloseProof, ProofAcknowledgement](OpProve, true),
	operation[ReconnectRequest, ReconnectResult](OpReconnect, true),
	operation[struct{}, Poll](OpPoll, false),
}
