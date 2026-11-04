package execution

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"reflect"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/scope/agent/interaction"
	"github.com/Tangerg/scope/core/chat"
)

func TestPreparedProviderInputSurvivesReloadUntilModelSettles(t *testing.T) {
	for _, unknown := range []bool{false, true} {
		name := "known response retires input"
		if unknown {
			name = "unknown response retains input"
		}
		t.Run(name, func(t *testing.T) {
			composition := &inputCaptureComposition{contextCaptureComposition: contextCaptureComposition{t: t}}
			var executor *InteractionExecutor
			var running *interactionSession
			var delivered *chat.Request
			model := chat.ModelFunc(func(ctx context.Context, request *chat.Request) (*chat.Response, error) {
				delivered = request.Clone()
				running = executor.sessions.snapshot()[0]
				head, found, err := running.executionTrees.LoadExecutionTree(ctx, running.start.SessionID, running.processRootID().String())
				if err != nil || !found {
					t.Fatalf("physical model has no durable preparation: %t, %v", found, err)
				}
				assertReloadedProviderInput(t, running, head, delivered)
				if unknown {
					return nil, errors.New("provider reply is unconfirmed")
				}
				return interactionUsageTextResponse("done", 1, 1), nil
			})
			executor = newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{CallComposition: composition})
			runInteractionHarness(t.Context(), t, executor, interactionTestStart(), nil)
			if delivered == nil || composition.transformed != 1 {
				t.Fatalf("provider preparation count=%d, request=%v", composition.transformed, delivered)
			}
			head, found, err := running.executionTrees.LoadExecutionTree(t.Context(), running.start.SessionID, running.processRootID().String())
			if err != nil || !found {
				t.Fatalf("settled execution has no durable head: %t, %v", found, err)
			}
			if unknown {
				assertReloadedProviderInput(t, running, head, delivered)
				return
			}
			var stored executionLogWire
			if err := json.Unmarshal(head.Payload, &stored); err != nil {
				t.Fatal(err)
			}
			if len(stored.PreparedRequests) != 0 {
				t.Fatal("acknowledged model settlement retained obsolete provider input")
			}
		})
	}
}

func assertReloadedProviderInput(t *testing.T, original *interactionSession, head runs.ExecutionTreeHead, delivered *chat.Request) {
	t.Helper()
	root := original.processRootID()
	history, err := decodeExecutionTree(head, root)
	if err != nil {
		t.Fatal(err)
	}
	// An empty local cache must restore through the actual Store boundary.
	reloaded := &interactionSession{executionTrees: original.executionTrees, start: original.start,
		material: newRequestMaterial(), accounting: newInteractionAccounting(original.start.ModelSelection)}
	reloaded.state.restoreTree = history
	loaded, found, err := reloaded.Load(t.Context(), root)
	if err != nil || !found {
		t.Fatalf("load captured provider input: %t, %v", found, err)
	}
	inspection, err := loaded.Inspect()
	if err != nil {
		t.Fatal(err)
	}
	process, found := inspection.Process(root)
	if !found {
		t.Fatal("restored log lost its process")
	}
	for _, pending := range process.PendingEffects {
		canonical, model, err := interaction.ModelRequestFromEffect(pending.Request.Effect())
		if err != nil {
			t.Fatal(err)
		}
		if !model {
			continue
		}
		if canonical.Messages[0].Text() != "current question" || canonical.Options.Temperature != nil {
			t.Fatal("provider transformation leaked into Scope's retained context")
		}
		reference, _, err := modelPreparationReference(canonical.Messages)
		if err != nil {
			t.Fatal(err)
		}
		source := modelCallSource{process: root, effect: pending.Request.ID(), program: process.Program, sequence: reference.Sequence}
		assertPreparedRequest(t, &reloaded.material, source, delivered)
		return
	}
	t.Fatal("restored log has no pending model input owner")
}

func assertPreparedRequest(t *testing.T, material *requestMaterial, source modelCallSource, delivered *chat.Request) {
	t.Helper()
	prepared, err := material.take(source)
	if err != nil || !reflect.DeepEqual(prepared.request, delivered) {
		t.Fatalf("restored provider input changed: %v", err)
	}
	prepared.request.Messages[0] = chat.NewUserMessage(chat.NewTextPart("mutated consumer copy"))
	independent, err := material.take(source)
	if err != nil || !reflect.DeepEqual(independent.request, delivered) {
		t.Fatalf("a consumer mutated retained provider input: %v", err)
	}
}
