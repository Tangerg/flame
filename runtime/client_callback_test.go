package runtime

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"net/http"
	"os"
	"os/exec"
	"testing"
	"time"

	runtimeclient "github.com/Tangerg/flame/packages/go-sdk/client"

	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	flamehttp "github.com/Tangerg/flame/runtime/internal/delivery/http"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

type coreCallbackOwnerKey struct{}

type coreCallbackTarget struct{ settled chan struct{} }

func (c *coreCallbackTarget) GetRunAcceptance(ctx context.Context, _ protocol.GetRunAcceptanceRequest) (*protocol.RunAcceptance, error) {
	if ctx.Value(coreCallbackOwnerKey{}) != "captured owner" {
		return nil, errors.New("captured owner was lost")
	}
	return &protocol.RunAcceptance{Type: protocol.RunAcceptanceNotRecorded}, nil
}

func (c *coreCallbackTarget) StartRun(context.Context, protocol.StartRunRequest) (*protocol.StartRunResponse, iter.Seq2[protocol.RunEvent, error], error) {
	return nil, nil, protocol.ErrSessionNotFound
}

func (c *coreCallbackTarget) SubscribeRuntime(ctx context.Context, _ protocol.RuntimeSubscribeRequest) (*protocol.RuntimeSubscribeResponse, iter.Seq2[protocol.RuntimeEvent, error], error) {
	return &protocol.RuntimeSubscribeResponse{}, func(func(protocol.RuntimeEvent, error) bool) {
		<-ctx.Done()
		close(c.settled)
	}, nil
}

type coreCallbackInput struct{ Host pluginsdk.CallbackRef }

func TestCoreCallbackBackendProcess(t *testing.T) {
	if os.Getenv("FLAME_CORE_CALLBACK_FIXTURE") != "1" {
		return
	}
	server := sdk.NewServer(&sdk.Implementation{Name: "core-callback-author"}, nil)
	backend, err := pluginsdk.NewBackend(server)
	if err == nil {
		err = pluginsdk.RegisterServer(backend, "test.backend/core", nil, func(ctx context.Context, _ *sdk.ServerSession, input coreCallbackInput) (string, error) {
			client, err := runtimeclient.ConnectCallback(ctx, input.Host)
			if err != nil {
				return "", err
			}
			defer client.Close()
			acceptance, err := client.GetRunAcceptance(ctx, protocol.GetRunAcceptanceRequest{Namespace: testsupport.IdempotencyNamespace, Key: "original", Operation: protocol.RunCommandStart}, protocol.CallOptions{})
			if err != nil || acceptance.Type != protocol.RunAcceptanceNotRecorded {
				return "", fmt.Errorf("original acceptance: %w", err)
			}
			_, err = client.GetRunAcceptance(ctx, protocol.GetRunAcceptanceRequest{Namespace: testsupport.IdempotencyNamespace, Key: "original", Operation: protocol.RunCommandStart}, protocol.CallOptions{RequestMeta: protocol.RequestMeta{ProtocolVersion: "1900-01-01"}})
			if !errors.Is(err, protocol.ErrInvalidProtocolVersion) {
				return "", fmt.Errorf("core protocol fence: %w", err)
			}
			_, _, err = client.StartRun(ctx, protocol.StartRunRequest{SessionID: "ses_absent", Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "go"}}}, protocol.RunCommandOptions{IdempotencyKey: "original", IdempotencyNamespace: testsupport.IdempotencyNamespace})
			if !errors.Is(err, protocol.ErrSessionNotFound) || errors.Is(err, runtimeclient.ErrAcknowledgementUnknown) {
				return "", fmt.Errorf("typed core refusal: %w", err)
			}
			_, _, err = client.SubscribeRuntime(ctx, protocol.RuntimeSubscribeRequest{Topics: []protocol.RuntimeTopic{protocol.TopicSessionsChanged}}, protocol.SubscriptionOptions{})
			return "canonical core query, refusal and stream", err
		})
	}
	if err == nil {
		err = backend.Publish()
	}
	if err == nil {
		err = server.Run(context.Background(), &sdk.StdioTransport{})
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(0)
}

func TestCoreCallbacksUseCanonicalDeliveryThroughARealBackend(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	callbacks := pluginsdk.NewCallbacks()
	t.Cleanup(func() {
		if err := callbacks.Close(); err != nil {
			t.Error(err)
		}
	})
	target := &coreCallbackTarget{settled: make(chan struct{})}
	endpoint, err := delivery.NewEndpoint(target, delivery.EndpointConfig{Lifetime: ctx, IdempotencyNamespace: testsupport.IdempotencyNamespace})
	if err != nil {
		t.Fatal(err)
	}
	rpc, err := flamehttp.NewRPCHandler(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	for _, route := range flamehttp.Contract().Endpoints {
		if route.Kind == flamehttp.EndpointKindRPC {
			mux.Handle(route.Method+" "+route.Path, rpc)
		}
	}
	if err := callbacks.PublishRPC(mux); err != nil {
		t.Fatal(err)
	}
	if err := callbacks.PublishRPC(mux); err == nil {
		t.Fatal("published RPC handler was replaced")
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "headless-host"}, nil)
	if err := pluginsdk.RegisterClient(client); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(os.Args[0], "-test.run=^TestCoreCallbackBackendProcess$")
	command.Env = append(os.Environ(), "FLAME_CORE_CALLBACK_FIXTURE=1")
	session, err := client.Connect(ctx, &sdk.CommandTransport{Command: command}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		callbacks.RevokeSession(session)
		if err := session.Close(); err != nil {
			t.Error(err)
		}
	})
	callbacks.AdmitSession(session)
	owner := context.WithValue(ctx, coreCallbackOwnerKey{}, "captured owner")
	ref, release, err := callbacks.OpenRPC(owner, session, pluginsdk.CallbackLimits{MaxRequestBytes: 4096, MaxResponseBytes: 4096})
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	result, err := pluginsdk.Call[coreCallbackInput, string](ctx, session, "test.backend/core", coreCallbackInput{Host: ref})
	if err != nil || result != "canonical core query, refusal and stream" {
		t.Fatalf("actual backend callback = %q, %v", result, err)
	}
	release()
	select {
	case <-target.settled:
	default:
		t.Fatal("callback retirement returned before the stream source joined")
	}
	stale, err := runtimeclient.ConnectCallback(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	defer stale.Close()
	_, _, err = stale.StartRun(ctx, protocol.StartRunRequest{SessionID: "ses_absent", Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "go"}}}, protocol.RunCommandOptions{})
	if !errors.Is(err, pluginsdk.ErrStaleCall) || errors.Is(err, runtimeclient.ErrAcknowledgementUnknown) {
		t.Fatalf("stale callback command = %v", err)
	}
	bounded, releaseBounded, err := callbacks.OpenRPC(owner, session, pluginsdk.CallbackLimits{MaxRequestBytes: 16, MaxResponseBytes: 4096})
	if err != nil {
		t.Fatal(err)
	}
	defer releaseBounded()
	limited, err := runtimeclient.ConnectCallback(ctx, bounded)
	if err != nil {
		t.Fatal(err)
	}
	defer limited.Close()
	_, _, err = limited.StartRun(ctx, protocol.StartRunRequest{SessionID: "ses_absent", Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "go"}}}, protocol.RunCommandOptions{})
	problem, known := errors.AsType[*runtimeclient.TransportError](err)
	if !known || problem.StatusCode != http.StatusRequestEntityTooLarge || errors.Is(err, runtimeclient.ErrAcknowledgementUnknown) {
		t.Fatalf("callback request capacity lost its canonical refusal: %v", err)
	}
}
