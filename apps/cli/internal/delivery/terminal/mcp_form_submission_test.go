package terminal

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/Tangerg/flame/apps/cli/internal/application/integration/mcp"
	"github.com/Tangerg/flame/apps/cli/internal/application/mutation"
	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/apps/cli/internal/runtimefixture"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/programtest"
)

type mcpFormAttempt struct {
	id        authoring.CommandID
	candidate mcp.Candidate
	update    mcp.ServerUpdate
}

type controlledMCPFormService struct {
	*mcpServiceStub
	attempts chan mcpFormAttempt
	replies  chan error
}

func newControlledMCPFormService() *controlledMCPFormService {
	return &controlledMCPFormService{mcpServiceStub: newMCPServiceStub(), attempts: make(chan mcpFormAttempt, 2), replies: make(chan error, 1)}
}

func (m *controlledMCPFormService) CreateServer(ctx context.Context, id authoring.CommandID, candidate mcp.Candidate) (protocol.MCPServer, error) {
	m.attempts <- mcpFormAttempt{id: id, candidate: candidate.Clone()}
	select {
	case err := <-m.replies:
		if err != nil {
			return protocol.MCPServer{}, err
		}
		return m.mcpServiceStub.CreateServer(ctx, id, candidate)
	case <-ctx.Done():
		return protocol.MCPServer{}, ctx.Err()
	}
}

func (m *controlledMCPFormService) UpdateServer(ctx context.Context, id authoring.CommandID, update mcp.ServerUpdate) (protocol.MCPServer, error) {
	m.attempts <- mcpFormAttempt{id: id, update: update}
	select {
	case err := <-m.replies:
		if err != nil {
			return protocol.MCPServer{}, err
		}
		return m.mcpServiceStub.UpdateServer(ctx, id, update)
	case <-ctx.Done():
		return protocol.MCPServer{}, ctx.Err()
	}
}

func submitMCPCreateForm(t *testing.T, host *programtest.Host) {
	t.Helper()
	host.Shows(t, "Ask flame")
	host.Type("/mcp-create")
	host.Press(input.Enter)
	host.Shows(t, "Create MCP server · 1/3")
	host.Type("private-docs")
	host.Press(input.Enter)
	host.Shows(t, "HTTP URL")
	host.Type("https://private.example/tools")
	host.Press(input.Enter)
	host.Shows(t, "Handshake timeout seconds")
	host.Press(input.Enter)
}

func TestMCPFormPendingSubmissionAndUnknownRetryUseOneOriginalCommand(t *testing.T) {
	service := newControlledMCPFormService()
	host, _ := runUIWithRuntimeServices(t, Config{RuntimeProfile: featuredTerminalProfile(t), OpenWorkbench: memoryTestWorkbench, Runtime: runtimefixture.New(), MCP: service})
	submitMCPCreateForm(t, host)
	original := awaitValue(t, service.attempts, "original MCP submission")
	host.Shows(t, "Submitting MCP form")
	host.Press(input.Enter)
	host.Press(input.Enter)
	host.Shows(t, "Submitting MCP form")
	select {
	case <-service.attempts:
		t.Fatal("pending form submitted another command")
	default:
	}
	service.replies <- conversation.ErrDisconnected
	host.Shows(t, "Enter retries the original request")
	host.Resize(80, 24)
	host.Press(input.Enter)
	retry := awaitValue(t, service.attempts, "original MCP retry")
	if original.id.Validate() != nil || !reflect.DeepEqual(retry, original) {
		t.Fatal("unknown submission changed its command identity or request")
	}
	service.replies <- nil
	host.Shows(t, "private-docs · disconnected")
}

func TestMCPFormDefiniteRefusalRestoresEditing(t *testing.T) {
	service := newControlledMCPFormService()
	host, _ := runUIWithRuntimeServices(t, Config{RuntimeProfile: featuredTerminalProfile(t), OpenWorkbench: memoryTestWorkbench, Runtime: runtimefixture.New(), MCP: service})
	submitMCPCreateForm(t, host)
	original := awaitValue(t, service.attempts, "refused MCP submission")
	service.replies <- errors.New("refused by runtime")
	host.Shows(t, "Create MCP server · 3/3")
	host.Shows(t, "refused by runtime")
	host.Press(input.Enter)
	next := awaitValue(t, service.attempts, "new submission after definite refusal")
	if next.id == original.id || !reflect.DeepEqual(next.candidate, original.candidate) {
		t.Fatal("definite refusal did not retain the editable original draft")
	}
	service.replies <- nil
	host.Shows(t, "private-docs · disconnected")
}

func TestMCPFormCancelRetiresOnlyItsOwnPresentation(t *testing.T) {
	service := newControlledMCPFormService()
	host, _ := runUIWithRuntimeServices(t, Config{RuntimeProfile: featuredTerminalProfile(t), OpenWorkbench: memoryTestWorkbench, Runtime: runtimefixture.New(), MCP: service})
	submitMCPCreateForm(t, host)
	awaitValue(t, service.attempts, "pending MCP submission")
	host.Shows(t, "Submitting MCP form")
	host.Press(input.Esc)
	host.Shows(t, "Ask flame")
	host.Type("/mcp-create")
	host.Press(input.Enter)
	host.Shows(t, "Create MCP server · 1/3")
	host.Type("replacement")
	service.replies <- nil
	awaitValue(t, service.created, "canceled form's accepted command")
	host.Shows(t, "Create MCP server · 1/3")
	host.Shows(t, "replacement")
}

func TestMCPFormFlowRetainsOriginalMutationUntilSettlement(t *testing.T) {
	for _, mode := range []mcpFormMode{mcpFormCreate, mcpFormUpdate} {
		t.Run(map[mcpFormMode]string{mcpFormCreate: "create", mcpFormUpdate: "update"}[mode], func(t *testing.T) {
			service := newControlledMCPFormService()
			flow := newMCPFormFlow(mode, service.servers[0])
			flow.step = mcpFormPolicy
			flow.draft.name, flow.draft.description = "docs", "original description"
			flow.draft.replaceConnection = true
			flow.draft.url = "https://private.example/tools"
			flow.draft.authorizationMode, flow.draft.authorization = formChangeSet, "Bearer retained-secret"
			flow.draft.headersMode, flow.draft.headers = formChangeSet, `{"X-Key":"retained-secret"}`
			policy := testReplayPolicy(t, featuredTerminalProfile(t))
			original, err := flow.prepare(service, policy)
			if err != nil {
				t.Fatal(err)
			}
			flow.admitted(original)
			result := make(chan error, 1)
			go func() { _, err := original.work(t.Context()); result <- err }()
			first := awaitValue(t, service.attempts, "first mutation")
			if flow.advance() || flow.back() {
				t.Fatal("pending submission remained editable")
			}
			service.replies <- context.DeadlineExceeded
			flow.settle(awaitValue(t, result, "unknown mutation result"))
			if flow.state != mcpFormUncertain || flow.draft.authorization == "" {
				t.Fatal("unknown outcome retired the original intent")
			}
			flow.draft.name, flow.draft.description, flow.draft.authorization = "different", "different", "different"
			replacement := newControlledMCPFormService()
			retry, err := flow.prepare(replacement, policy)
			if err != nil || retry != original {
				t.Fatal("retry replaced its original submission")
			}
			flow.admitted(retry)
			go func() { _, err := retry.work(t.Context()); result <- err }()
			second := awaitValue(t, service.attempts, "retained mutation")
			if !reflect.DeepEqual(second, first) {
				t.Fatal("retry changed original target, input or command identity")
			}
			service.replies <- nil
			flow.settle(awaitValue(t, result, "confirmed mutation"))
			if flow.state != mcpFormClosed || flow.submission != nil || flow.draft.authorization != "" || flow.draft.headers != "" {
				t.Fatal("confirmed mutation retained submission secrets")
			}
		})
	}
}

func TestMCPFormExpiredReplayCannotBecomeANewCommand(t *testing.T) {
	service := newControlledMCPFormService()
	profile := featuredTerminalProfile(t)
	now := time.Now()
	policy, err := mutation.PolicyFromProfile(profile, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	flow := newMCPFormFlow(mcpFormCreate, protocol.MCPServer{})
	flow.draft.name, flow.draft.url = "docs", "https://private.example/tools"
	submission, err := flow.prepare(service, policy)
	if err != nil {
		t.Fatal(err)
	}
	flow.admitted(submission)
	flow.settle(context.DeadlineExceeded)
	now = now.Add(time.Duration(profile.IdempotencyLimits().RetentionSeconds+1) * time.Second)
	retry, err := flow.prepare(service, policy)
	if err != nil || retry != submission {
		t.Fatal("expired intent acquired another submission")
	}
	flow.admitted(retry)
	_, err = retry.work(t.Context())
	flow.settle(err)
	if !errors.Is(err, mutation.ErrReplayGuaranteeUnavailable) || flow.state != mcpFormBlocked {
		t.Fatal("expired replay was not fenced")
	}
	if retry, err := flow.prepare(service, policy); retry != nil || err != nil {
		t.Fatal("blocked outcome admitted another command")
	}
	select {
	case <-service.attempts:
		t.Fatal("expired replay reached Runtime")
	default:
	}
	flow.close()
	if flow.state != mcpFormClosed || flow.submission != nil {
		t.Fatal("explicit cancellation retained blocked submission")
	}
}

func TestMCPFormCancellationClearsSecretsWithoutChangingAdmittedInput(t *testing.T) {
	service := newControlledMCPFormService()
	flow := newMCPFormFlow(mcpFormCreate, protocol.MCPServer{})
	flow.draft.name, flow.draft.url = "docs", "https://private.example/tools"
	flow.draft.authorizationMode, flow.draft.authorization = formChangeSet, "Bearer retained-secret"
	flow.draft.headersMode, flow.draft.headers = formChangeSet, `{"X-Key":"retained-secret"}`
	submission, err := flow.prepare(service, testReplayPolicy(t, featuredTerminalProfile(t)))
	if err != nil {
		t.Fatal(err)
	}
	flow.admitted(submission)
	result := make(chan error, 1)
	go func() { _, err := submission.work(t.Context()); result <- err }()
	original := awaitValue(t, service.attempts, "admitted input")
	flow.close()
	if flow.draft.authorization != "" || flow.draft.headers != "" || flow.submission != nil {
		t.Fatal("explicit cancellation retained secrets")
	}
	service.replies <- nil
	flow.settle(awaitValue(t, result, "canceled form settlement"))
	admitted := awaitValue(t, service.created, "original admitted candidate")
	if !reflect.DeepEqual(admitted, original.candidate) || admitted.Connection.Authorization.Value != "Bearer retained-secret" {
		t.Fatal("form cleanup changed the already admitted input")
	}
	if flow.state != mcpFormClosed {
		t.Fatal("late settlement reopened a canceled form")
	}
}
