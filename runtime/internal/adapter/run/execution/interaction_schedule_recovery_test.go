package execution

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	domaintool "github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/fingerprint"
	"github.com/Tangerg/flame/runtime/internal/infra/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

type scheduleReceiptTool struct {
	toolcontract.Tool
	configuration mcp.ServerConfig
}

func (s scheduleReceiptTool) Unwrap() toolcontract.Tool      { return s.Tool }
func (s scheduleReceiptTool) SourceConfig() mcp.ServerConfig { return s.configuration }
func (s scheduleReceiptTool) SourceFingerprint() fingerprint.Digest {
	return s.configuration.AuthorityFingerprint()
}
func (s scheduleReceiptTool) MCPToolIdentity() (string, string) {
	return s.configuration.Name.String(), "create_schedule"
}

type scheduleReceiptProbe struct {
	owned   bool
	query   func(context.Context) (*chat.ToolOutput, error)
	calls   atomic.Int32
	context atomic.Pointer[context.Context]
}

func (*scheduleReceiptProbe) AwaitOriginalCreateSource(context.Context, domaintool.Ref, fingerprint.Digest, fingerprint.Digest) error {
	return nil
}
func (s *scheduleReceiptProbe) OwnsCreateSource(context.Context, domaintool.Ref, fingerprint.Digest) (bool, error) {
	return s.owned, nil
}
func (s *scheduleReceiptProbe) OriginalCreateResult(ctx context.Context, _ domaintool.Ref, _, _ fingerprint.Digest, _, _ string) (*chat.ToolOutput, error) {
	s.calls.Add(1)
	s.context.Store(new(ctx))
	return s.query(ctx)
}

type schedulePublicationTrees struct {
	runs.ExecutionTreeStore
	probe    *scheduleReceiptProbe
	observed atomic.Bool
}

func (s *schedulePublicationTrees) SaveExecutionTree(ctx context.Context, update runs.ExecutionTreeUpdate) error {
	if query := s.probe.context.Load(); query != nil && (*query).Err() != nil {
		if ctx.Err() != nil {
			return errors.New("query lifetime canceled result publication")
		}
		s.observed.Store(true)
	}
	return s.ExecutionTreeStore.SaveExecutionTree(ctx, update)
}

func TestScheduleInvestigationRespectsCancellationTimeoutAndLaterUnknown(t *testing.T) {
	for _, mode := range []string{"cancel", "timeout", "unavailable", "later model", "publication", "other service"} {
		t.Run(mode, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				installation := testsupport.InstallationID(t)
				source, err := mcpserver.InstallationSource(installation, testsupport.Digest("schedule release"), testsupport.Digest("schedule authority"), testsupport.Digest("schedule recipient"))
				if err != nil {
					t.Fatal(err)
				}
				configuration := mcp.ServerConfig{Server: mcpserver.Server{Source: source, Name: testsupport.ServerName("schedules"), Transport: mcpserver.TransportStdio, Command: "schedule-test", Enabled: true}}
				var toolCalls, modelCalls atomic.Int32
				inner, err := toolcontract.NewFunc(toolcontract.FuncConfig{Name: "schedules_create_schedule", Description: "Create one schedule."}, func(context.Context, struct {
					Title string `json:"title"`
				}) (string, error) {
					toolCalls.Add(1)
					return "", errors.New("reply lost after create")
				})
				if err != nil {
					t.Fatal(err)
				}
				entered, release := make(chan struct{}), make(chan struct{})
				probe := &scheduleReceiptProbe{owned: mode != "other service", query: func(ctx context.Context) (*chat.ToolOutput, error) {
					close(entered)
					if mode == "timeout" {
						<-ctx.Done()
						return nil, ctx.Err()
					}
					if mode == "unavailable" {
						return nil, errors.New("backend unavailable")
					}
					if mode == "cancel" {
						<-release
					}
					output, err := chat.NewJSONToolOutput([]byte(`{"schedule":{"schedule_id":"original","title":"original"}}`))
					return &output, err
				}}
				model := chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
					if modelCalls.Add(1) == 1 {
						return interactionToolResponse(chat.ToolCall{ID: "create", Name: "schedules_create_schedule", Arguments: `{"title":"original"}`}, 1, 1), nil
					}
					if mode == "publication" {
						return interactionUsageTextResponse("completed", 1, 1), nil
					}
					return nil, errors.New("later model outcome is unknown")
				})
				trees := &schedulePublicationTrees{ExecutionTreeStore: testTrees(t), probe: probe}
				executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{ExecutionTrees: trees, OriginalScheduleResults: probe, ToolResolver: staticInteractionTools{manifest: toolset.Manifest{Visible: []toolcontract.Tool{scheduleReceiptTool{Tool: inner, configuration: configuration}}}}, ToolInterpreter: immutableToolInterpreter{}, ToolAuthorizer: allowInteractionTools{}})
				ref, err := executor.StageRoot(t.Context(), interactionTestStart())
				if err != nil {
					t.Fatal(err)
				}
				defer func() {
					if err := executor.Release(context.Background(), ref); err != nil {
						t.Error(err)
					}
				}()
				session, _ := executor.session(ref)
				session.lifetime.unknownWake = nil
				session.unknownPollInterval = time.Hour
				sequence, err := observeTestInteraction(t, executor, t.Context(), ref)
				if err != nil {
					t.Fatal(err)
				}
				events := collectInteractionEvents(sequence)
				if err := executor.BeginRoot(t.Context(), ref); err != nil {
					t.Fatal(err)
				}
				synctest.Wait()
				reported := make(chan bool, 1)
				go func() { reported <- session.reportUnknownEffects() }()
				if mode != "other service" {
					<-entered
				}
				if mode == "cancel" {
					if err := executor.RequestRootCancellation(t.Context(), ref, "operator canceled"); err != nil {
						t.Fatal(err)
					}
					synctest.Wait()
					close(release)
				} else if mode == "timeout" {
					time.Sleep(auxiliaryOperationTimeout)
				}
				<-reported
				ends := payloadsOf[runs.SegmentEnded](<-events)
				want := run.OutcomeLost
				if mode == "cancel" {
					want = run.OutcomeCanceled
				}
				if mode == "later model" {
					want = run.OutcomeFailed
				}
				if mode == "publication" {
					want = run.OutcomeCompleted
				}
				if len(ends) != 1 || ends[0].Reason != want {
					t.Fatalf("terminal=%+v, want %s", ends, want)
				}
				queries := int32(1)
				if mode == "other service" {
					queries = 0
				}
				if toolCalls.Load() != 1 || probe.calls.Load() != queries {
					t.Fatalf("external calls=%d queries=%d", toolCalls.Load(), probe.calls.Load())
				}
				if mode == "later model" && modelCalls.Load() != 2 {
					t.Fatalf("later model calls=%d", modelCalls.Load())
				}
				if mode == "later model" {
					session.state.mu.Lock()
					for _, metadata := range session.state.toolMetadata {
						if metadata.Create != nil {
							t.Error("a later model call retained the old create recovery prefix")
						}
					}
					session.state.mu.Unlock()
				}
				uncertain := 1
				if mode == "publication" {
					uncertain = 0
					if !trees.observed.Load() {
						t.Fatal("no publication outlived its query context")
					}
				}
				if len(ends[0].UnresolvedEffects()) != uncertain {
					t.Fatalf("uncertain evidence=%+v", ends[0].UnresolvedEffects())
				}
			})
		})
	}
}
