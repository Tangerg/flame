package workbench_test

import (
	"context"
	"errors"
	"io/fs"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/apps/cli/internal/runtimefixture"

	"github.com/Tangerg/flame/apps/cli/internal/application/mutation"
	"github.com/Tangerg/flame/apps/cli/internal/application/workbench"
	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/qimu/portable"
)

type persistence struct {
	files map[string][]byte
	fail  bool
}

func (p *persistence) Read(name string, _ int64) ([]byte, error) {
	value, found := p.files[name]
	if !found {
		return nil, fs.ErrNotExist
	}
	return slices.Clone(value), nil
}
func (*persistence) ListFiles(string, string) ([]string, error) { return nil, nil }
func (p *persistence) Replace(name string, body []byte) error {
	if p.fail {
		return errors.New("storage unavailable")
	}
	p.files[name] = slices.Clone(body)
	return nil
}
func (p *persistence) Remove(name string) error { delete(p.files, name); return nil }

type callerFunc func(context.Context, protocol.CallPluginRequest, authoring.CommandID) (*protocol.CallPluginResponse, error)

func (f callerFunc) CallPlugin(ctx context.Context, request protocol.CallPluginRequest, id authoring.CommandID) (*protocol.CallPluginResponse, error) {
	return f(ctx, request, id)
}

func policy(t *testing.T, namespace string, now *time.Time) mutation.ReplayPolicy {
	t.Helper()
	capability, err := authoring.NewReplayCapability(namespace, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	result, err := mutation.PolicyFromProfile(runtimefixture.ReplayProfile{Capability: capability}, func() time.Time { return *now })
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func request(t *testing.T) protocol.CallPluginRequest {
	t.Helper()
	contract, err := portable.NewCapability(portable.CapabilityConfig{Reference: "schedules", Composition: portable.Selection, Scope: "installation"}, portable.MethodDescriptor{Name: "flame.schedules/delete.v1", Kind: portable.Mutation, Target: "installation", Description: "Delete a schedule", Input: []byte(`{"type":"object","properties":{"id":{"type":"string"}},"required":["id"],"additionalProperties":false}`), Output: []byte(`{"type":"object","properties":{},"additionalProperties":false}`)})
	if err != nil {
		t.Fatal(err)
	}
	return protocol.CallPluginRequest{InstallationID: "12345678-1234-1234-1234-123456789abc", Digest: strings.Repeat("1", 64), Contribution: "schedules-provider", Contract: string(contract.Identity().JSON()), Method: "flame.schedules/delete.v1", Target: portable.Target{Scope: "installation", Reference: "12345678-1234-1234-1234-123456789abc"}, Input: `{"id":"sch_original"}`}
}

func execute(ctx context.Context, caller workbench.InstalledCaller, store *workbench.Store, intent string, policy mutation.ReplayPolicy, prepare func(context.Context) (protocol.CallPluginRequest, error)) error {
	_, err := workbench.ExecuteInstalledCommand(ctx, caller, store, intent, policy, prepare, func(*protocol.CallPluginResponse) (struct{}, error) { return struct{}{}, nil })
	return err
}

func TestInstalledCommandRejectsInvalidCapturedInputBeforePersistence(t *testing.T) {
	for _, input := range []string{`{}`, `{"id":null}`, `{"id":42}`} {
		t.Run(input, func(t *testing.T) {
			storage := &persistence{files: make(map[string][]byte)}
			store, err := workbench.Open(storage, workbench.Config{})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = store.Close() })
			invalid := request(t)
			invalid.Input = input
			now := time.Now()
			caller := callerFunc(func(context.Context, protocol.CallPluginRequest, authoring.CommandID) (*protocol.CallPluginResponse, error) {
				t.Fatal("invalid captured input reached the mutation owner")
				return nil, nil
			})
			prepare := func(context.Context) (protocol.CallPluginRequest, error) { return invalid, nil }
			if err := execute(t.Context(), caller, store, "delete", policy(t, "store", &now), prepare); err == nil {
				t.Fatal("invalid captured input was accepted")
			}
			if len(storage.files) != 0 || len(store.PendingInstalledCommands()) != 0 {
				t.Fatal("invalid captured input was retained as durable intent")
			}
		})
	}
}

func TestLostAcknowledgementResumesOriginalRequestAfterRestart(t *testing.T) {
	storage := &persistence{files: make(map[string][]byte)}
	store, err := workbench.Open(storage, workbench.Config{})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)
	original := request(t)
	prepare := func(context.Context) (protocol.CallPluginRequest, error) { return original, nil }
	ctx, cancel := context.WithCancel(t.Context())
	var capturedID authoring.CommandID
	lost := callerFunc(func(_ context.Context, input protocol.CallPluginRequest, id authoring.CommandID) (*protocol.CallPluginResponse, error) {
		pending, staged := store.PendingInstalledCommand("schedule-delete:sch_original")
		if !staged || pending.CommandID != id || !(input.Input == original.Input) {
			t.Fatal("command dispatched before durable intent")
		}
		capturedID = id
		input.Input = " " + input.Input[1:]
		cancel()
		return nil, conversation.ErrDisconnected
	})
	if err := execute(ctx, lost, store, "schedule-delete:sch_original", policy(t, "original-store", &now), prepare); err == nil {
		t.Fatal("lost acknowledgement became success")
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := workbench.Open(storage, workbench.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	prepare = func(context.Context) (protocol.CallPluginRequest, error) {
		t.Fatal("pending command rediscovered the current provider")
		return protocol.CallPluginRequest{}, nil
	}
	calls := 0
	recover := callerFunc(func(_ context.Context, input protocol.CallPluginRequest, id authoring.CommandID) (*protocol.CallPluginResponse, error) {
		calls++
		if id != capturedID || input.InstallationID != original.InstallationID || input.Digest != original.Digest || input.Contract != original.Contract || input.Method != original.Method || input.Contribution != original.Contribution || input.Target != original.Target || !(input.Input == original.Input) {
			t.Fatal("recovery changed the captured route, input or identity")
		}
		return &protocol.CallPluginResponse{Output: `{}`}, nil
	})
	if err := execute(t.Context(), recover, reopened, "schedule-delete:sch_original", policy(t, "other-store", &now), prepare); err == nil || calls != 0 {
		t.Fatal("store mismatch dispatched or confirmed the original command")
	}
	if err := execute(t.Context(), recover, reopened, "schedule-delete:sch_original", policy(t, "original-store", &now), prepare); err != nil || calls != 1 {
		t.Fatalf("recover original receipt: calls=%d, error=%v", calls, err)
	}
	if _, pending := reopened.PendingInstalledCommand("schedule-delete:sch_original"); pending {
		t.Fatal("acknowledged command remains pending")
	}
}

func TestFailedPersistenceAndUnavailableOwnerCannotDiscardIntent(t *testing.T) {
	storage := &persistence{files: make(map[string][]byte), fail: true}
	store, err := workbench.Open(storage, workbench.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)
	prepare := func(context.Context) (protocol.CallPluginRequest, error) { return request(t), nil }
	calls := 0
	owner := callerFunc(func(context.Context, protocol.CallPluginRequest, authoring.CommandID) (*protocol.CallPluginResponse, error) {
		calls++
		return nil, protocol.ErrPluginUnavailable
	})
	if err := execute(t.Context(), owner, store, "delete", policy(t, "store", &now), prepare); err == nil || calls != 0 {
		t.Fatal("failed durable staging dispatched the command")
	}
	storage.fail = false
	if err := execute(t.Context(), owner, store, "delete", policy(t, "store", &now), prepare); err == nil || calls != 1 {
		t.Fatal("unavailable original owner became success")
	}
	if _, pending := store.PendingInstalledCommand("delete"); !pending {
		t.Fatal("unavailable original owner lost its pending intent")
	}
	now = now.Add(2 * time.Hour)
	if err := execute(t.Context(), owner, store, "delete", policy(t, "store", &now), prepare); err == nil || calls != 1 {
		t.Fatal("expired replay guarantee dispatched a fresh command")
	}
}

func TestOnlyOwnerEvidenceRetiresTheOriginalIntent(t *testing.T) {
	storage := &persistence{files: make(map[string][]byte)}
	store, err := workbench.Open(storage, workbench.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)
	prepare := func(context.Context) (protocol.CallPluginRequest, error) { return request(t), nil }
	var originalID authoring.CommandID
	owner := callerFunc(func(_ context.Context, _ protocol.CallPluginRequest, id authoring.CommandID) (*protocol.CallPluginResponse, error) {
		if originalID != "" && id != originalID {
			t.Fatal("cleanup failure allocated another command")
		}
		originalID = id
		storage.fail = true
		return &protocol.CallPluginResponse{Output: `{}`}, nil
	})
	if err := execute(t.Context(), owner, store, "delete", policy(t, "store", &now), prepare); err == nil {
		t.Fatal("failed retirement became complete cleanup")
	}
	if _, pending := store.PendingInstalledCommand("delete"); !pending {
		t.Fatal("failed retirement lost the original identity")
	}
	storage.fail = false
	owner = callerFunc(func(_ context.Context, _ protocol.CallPluginRequest, id authoring.CommandID) (*protocol.CallPluginResponse, error) {
		if id != originalID {
			t.Fatal("owner receipt was queried under another identity")
		}
		return &protocol.CallPluginResponse{Output: `{}`}, nil
	})
	if err := execute(t.Context(), owner, store, "delete", policy(t, "store", &now), prepare); err != nil {
		t.Fatal(err)
	}
	if _, pending := store.PendingInstalledCommand("delete"); pending {
		t.Fatal("confirmed owner receipt remains unresolved")
	}
	refusal := &portable.Refusal{Code: "revision_conflict", Message: "schedule changed"}
	owner = callerFunc(func(_ context.Context, _ protocol.CallPluginRequest, id authoring.CommandID) (*protocol.CallPluginResponse, error) {
		if id == originalID {
			t.Fatal("new intent reused the original identity")
		}
		return nil, refusal
	})
	if err := execute(t.Context(), owner, store, "refused", policy(t, "store", &now), prepare); !errors.Is(err, refusal) {
		t.Fatalf("owner refusal: %v", err)
	}
	if _, pending := store.PendingInstalledCommand("refused"); pending {
		t.Fatal("definite owner refusal remains unresolved")
	}
}

func TestDecodedAcknowledgementPrecedesIntentRetirement(t *testing.T) {
	store, err := workbench.OpenMemory(workbench.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	now := time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)
	preparations := 0
	prepare := func(context.Context) (protocol.CallPluginRequest, error) {
		preparations++
		return request(t), nil
	}
	var originalID authoring.CommandID
	owner := callerFunc(func(_ context.Context, _ protocol.CallPluginRequest, id authoring.CommandID) (*protocol.CallPluginResponse, error) {
		if originalID != "" && id != originalID {
			t.Fatal("invalid acknowledgement replaced the original command")
		}
		originalID = id
		return &protocol.CallPluginResponse{Output: `{}`}, nil
	})
	decode := func(*protocol.CallPluginResponse) (string, error) {
		return "", &portable.Refusal{Code: "invalid_projection", Message: "invalid owner acknowledgement"}
	}
	if _, err := workbench.ExecuteInstalledCommand(t.Context(), owner, store, "accept", policy(t, "store", &now), prepare, decode); err == nil {
		t.Fatal("invalid decoded acknowledgement became success")
	}
	if _, pending := store.PendingInstalledCommand("accept"); !pending {
		t.Fatal("invalid decoded acknowledgement retired the original intent")
	}
	decode = func(*protocol.CallPluginResponse) (string, error) { return "original acceptance", nil }
	result, err := workbench.ExecuteInstalledCommand(t.Context(), owner, store, "accept", policy(t, "store", &now), prepare, decode)
	if err != nil || result != "original acceptance" || preparations != 1 {
		t.Fatalf("decoded original acceptance: %q, preparations=%d, error=%v", result, preparations, err)
	}
	if _, pending := store.PendingInstalledCommand("accept"); pending {
		t.Fatal("validated acknowledgement remains unresolved")
	}
}
