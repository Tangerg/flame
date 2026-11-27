package runtime_test

import (
	"context"
	"crypto/rand"
	json "encoding/json/v2"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/packages/go-sdk/runevent"
	flameruntime "github.com/Tangerg/flame/runtime"
	"github.com/Tangerg/flame/runtime/internal/testsupport"

	framecall "github.com/Tangerg/frame/call"

	framevalue "github.com/Tangerg/frame/value"
)

type liveDeepSeekFixture struct {
	ctx               context.Context
	runtime           *flameruntime.Runtime
	workspace         string
	dataDirectory     string
	userHome          string
	configDirectories []string
}

func newLiveDeepSeekFixture(t *testing.T, timeout time.Duration) *liveDeepSeekFixture {
	t.Helper()
	if os.Getenv(liveDeepSeekEnvironment) != "1" {
		t.Skipf("set %s=1 to run paid live DeepSeek E2E", liveDeepSeekEnvironment)
	}
	configDirectory := resolveLiveConfigDirectory(t)
	workspace, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), timeout)
	t.Cleanup(cancel)
	fixture := &liveDeepSeekFixture{
		ctx:               ctx,
		workspace:         workspace,
		dataDirectory:     filepath.Join(testsupport.TempDirectory(t), "runtime"),
		userHome:          testsupport.TempDirectory(t),
		configDirectories: []string{configDirectory},
	}
	runtime, err := flameruntime.Open(ctx, fixture.config())
	if err != nil {
		t.Fatal(err)
	}
	fixture.runtime = runtime
	t.Cleanup(func() {
		var closeErr error
		for range 3 {
			closeErr = fixture.runtime.Close()
			if closeErr == nil || errors.Is(closeErr, flameruntime.ErrClosed) {
				return
			}
		}
		t.Errorf("close live Runtime: %v", closeErr)
	})
	return fixture
}

type liveCrashState struct {
	SessionID string `json:"sessionId"`
	RunID     string `json:"runId"`
}

func resolveLiveConfigDirectory(t *testing.T) string {
	t.Helper()
	directory := os.Getenv(liveConfigDirectoryEnvironment)
	if directory == "" {
		directory = "config"
	}
	resolved, err := filepath.Abs(directory)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func requiredLiveCrashPath(t *testing.T, name string) string {
	t.Helper()
	value := os.Getenv(name)
	if value == "" || !filepath.IsAbs(value) {
		t.Fatalf("%s must be a non-empty absolute path", name)
	}
	return filepath.Clean(value)
}

func writeLiveCrashState(t *testing.T, path string, state liveCrashState) {
	t.Helper()
	content, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, content, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(temporary, path); err != nil {
		t.Fatal(err)
	}
}

func waitForLiveCrashFile(
	t *testing.T,
	ctx context.Context,
	path string,
	helperDone <-chan error,
) []byte {
	t.Helper()
	for {
		content, err := os.ReadFile(path)
		if err == nil {
			return content
		} else if !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		select {
		case <-ctx.Done():
			t.Fatalf("waiting for live crash file %s: %v", filepath.Base(path), ctx.Err())
		case err := <-helperDone:
			t.Fatalf("live crash helper exited before publishing %s: %v", filepath.Base(path), err)
		case <-time.After(25 * time.Millisecond):
		}
	}
}

func assertRecoveredShellIncomplete(t *testing.T, runtime *flameruntime.Runtime, ctx context.Context, runID string) {
	t.Helper()
	limit := 100
	items, err := runtime.ListItems(ctx, protocol.ListItemsRequest{
		Scope:     protocol.ItemListScope{Type: protocol.ItemScopeRun, RunID: runID},
		PageQuery: protocol.PageQuery{Limit: &limit},
	}, protocol.CallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items.Data {
		if item.Type == protocol.ItemTypeToolCall && item.Tool != nil && item.Tool.Name == "shell" {
			if item.Status != protocol.ItemStatusIncomplete {
				t.Fatalf("recovered shell Item = %+v, want incomplete", item)
			}
			return
		}
	}
	t.Fatalf("recovered Run %s has no shell Item", runID)
}

func (f *liveDeepSeekFixture) config() flameruntime.Config {
	return flameruntime.Config{
		DataDirectory:        f.dataDirectory,
		DefaultWorkspacePath: f.workspace,
		UserHomePath:         f.userHome,
		ConfigDirectories:    f.configDirectories,
	}
}

func (f *liveDeepSeekFixture) restart(t *testing.T) {
	t.Helper()
	if err := f.runtime.Close(); err != nil {
		t.Fatal(err)
	}
	runtime, err := flameruntime.Open(f.ctx, f.config())
	if err != nil {
		t.Fatal(err)
	}
	f.runtime = runtime
}

func (f *liveDeepSeekFixture) pendingQuestion(
	t *testing.T,
	sessionID, runID string,
	requestMeta protocol.RequestMeta,
) protocol.PendingInterruptSet {
	t.Helper()
	interrupts, err := f.runtime.ListInterrupts(f.ctx, protocol.ListInterruptsRequest{
		SessionID: sessionID,
	}, protocol.CallOptions{RequestMeta: requestMeta})
	if err != nil {
		t.Fatal(err)
	}
	if len(interrupts.Data) != 1 || interrupts.Data[0].RootRunID != runID ||
		len(interrupts.Data[0].Interrupts) != 1 ||
		interrupts.Data[0].Interrupts[0].Type != protocol.InterruptQuestion ||
		interrupts.Data[0].Interrupts[0].Payload == nil ||
		interrupts.Data[0].Interrupts[0].Payload.Question == nil {
		t.Fatalf("pending interrupts = %+v, want one question for Run %s", interrupts.Data, runID)
	}
	return interrupts.Data[0]
}

func (f liveDeepSeekFixture) createSession(t *testing.T, title string) *protocol.Session {
	t.Helper()
	session, err := f.runtime.CreateSession(f.ctx, protocol.CreateSessionRequest{
		Workspace: &protocol.WorkspaceRef{Path: f.workspace},
		Title:     title,
	}, protocol.CommandOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return session
}

type liveTurn struct {
	finalText      string
	compactionSeen bool
	run            *protocol.RunRef
}

func (f liveDeepSeekFixture) runTurn(t *testing.T, sessionID, prompt string) liveTurn {
	t.Helper()

	started, events, err := f.runtime.StartRun(f.ctx, protocol.StartRunRequest{
		SessionID: sessionID,
		Input:     []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: prompt}},
	}, protocol.RunCommandOptions{IdempotencyKey: rand.Text()})
	if err != nil {
		t.Fatal(err)
	}
	compactionSeen := false
	var boundaryContextTokens *int64
	for event, eventErr := range events {
		if eventErr != nil {
			t.Fatal(eventErr)
		}
		if event.Event.Type == runevent.ItemCompleted && event.Event.Item != nil &&
			event.Event.Item.Type == protocol.ItemTypeCompaction {
			compactionSeen = true
		}
		if event.RunID == started.RunID && event.Event.Type == runevent.SegmentFinished {
			contextTokens := event.Event.Run.ContextTokens
			boundaryContextTokens = &contextTokens
		}
	}
	run := f.getCompletedRun(t, started.RunID)
	if boundaryContextTokens == nil {
		t.Fatalf("Run %s stream omitted its final segment boundary", started.RunID)
	}
	if *boundaryContextTokens != run.ContextTokens {
		t.Fatalf("Run %s final stream context = %d, cold Run context = %d", started.RunID, *boundaryContextTokens, run.ContextTokens)
	}
	return liveTurn{finalText: f.finalAnswer(t, started.RunID), compactionSeen: compactionSeen, run: run}
}

func (f liveDeepSeekFixture) getCompletedRun(t *testing.T, runID string) *protocol.RunRef {
	t.Helper()
	run, err := f.runtime.GetRun(f.ctx, protocol.GetRunRequest{RunID: runID}, protocol.CallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != protocol.RunStatusFinished || run.Outcome == nil || run.Outcome.Type != protocol.OutcomeCompleted {
		t.Fatalf("Run %s = %+v, want completed", runID, run)
	}
	return run
}

func (f liveDeepSeekFixture) finalAnswer(t *testing.T, runID string) string {
	t.Helper()
	limit := 100
	items, err := f.runtime.ListItems(f.ctx, protocol.ListItemsRequest{
		Scope:     protocol.ItemListScope{Type: protocol.ItemScopeRun, RunID: runID},
		PageQuery: protocol.PageQuery{Limit: &limit},
	}, protocol.CallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for index := len(items.Data) - 1; index >= 0; index-- {
		item := items.Data[index]
		if item.Type != protocol.ItemTypeAgentMessage || item.Phase != protocol.MessagePhaseFinalAnswer {
			continue
		}
		var text strings.Builder
		for _, block := range item.Content {
			if block.Type == protocol.ContentBlockText {
				text.WriteString(block.Text)
			}
		}
		return text.String()
	}
	t.Fatalf("Run %s has no final AgentMessage", runID)
	return ""
}

func assertDeepSeekRun(t *testing.T, run *protocol.RunRef) {
	t.Helper()
	if run.Provider != "deepseek" || run.Model == "" || run.Metrics.Usage == nil ||
		run.Metrics.Usage.CostUSD == nil || *run.Metrics.Usage.CostUSD <= 0 {
		t.Fatalf("Run did not use metered DeepSeek: %+v", run)
	}
}

func fixtureScheduleRunRequest(t *testing.T, fixture *liveDeepSeekFixture, id string) protocol.CallPluginRequest {
	t.Helper()
	providers, err := fixture.runtime.ListPluginProviders(fixture.ctx, protocol.CallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	installations, err := fixture.runtime.ListPlugins(fixture.ctx, protocol.CallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	contracts, err := plugincontract.ParseCapability([]byte(testsupport.SchedulesCapabilityJSON))
	if err != nil {
		t.Fatal(err)
	}
	input, err := json.Marshal(testsupport.SchedulesItemInput{Id: id})
	if err != nil {
		t.Fatal(err)
	}
	for _, provider := range providers.Data {
		if provider.Role != "schedules" {
			continue
		}
		for _, installation := range installations.Data {
			if installation.ID == provider.InstallationID {
				return protocol.CallPluginRequest{
					Route: protocol.PluginCallRoute{
						InstallationID: installation.ID,
						Digest:         installation.Selected.Digest,
						Contribution:   provider.Contribution,
						Contract:       string(contracts.Identity().Bytes()),
						Method:         testsupport.SchedulesRunNowV2Method,
						Target:         framecall.Target{Scope: extension.ScopeInstallation, Reference: installation.ID},
					},
					Input: string(input),
				}
			}
		}
	}
	t.Fatal("selected schedules contribution unavailable")
	return protocol.CallPluginRequest{}
}

func fixtureScheduleRun(fixture *liveDeepSeekFixture, request protocol.CallPluginRequest, options protocol.PluginCallOptions) (*testsupport.SchedulesRunReference, error) {
	reply, err := fixture.runtime.CallPlugin(fixture.ctx, request, options)
	if err != nil {
		return nil, err
	}
	if reply.Failure != nil {
		return nil, &framecall.Refusal{Code: reply.Failure.Code, Message: reply.Failure.Message}
	}
	var value testsupport.SchedulesRunReference
	if err := json.Unmarshal([]byte(reply.Output), &value, json.RejectUnknownMembers(true)); err != nil {
		return nil, err
	}
	return &value, nil
}

func fixtureScheduleCreateRequest(t *testing.T, fixture *liveDeepSeekFixture, draft testsupport.SchedulesDraft) protocol.CallPluginRequest {
	t.Helper()
	request := fixtureScheduleRunRequest(t, fixture, "")
	input, err := json.Marshal(draft)
	if err != nil {
		t.Fatal(err)
	}
	request.Route.Method, request.Input = testsupport.SchedulesCreateV2Method, string(input)
	return request
}

func fixtureScheduleCreate(fixture *liveDeepSeekFixture, request protocol.CallPluginRequest) (*testsupport.SchedulesCreateV2OutputScheduleSnapshot, error) {
	reply, err := fixture.runtime.CallPlugin(fixture.ctx, request, protocol.PluginCallOptions{IdempotencyKey: "live-deepseek-create-schedule"})
	if err != nil {
		return nil, err
	}
	if reply.Failure != nil {
		return nil, &framecall.Refusal{Code: reply.Failure.Code, Message: reply.Failure.Message}
	}
	var value testsupport.SchedulesCreateV2OutputScheduleSnapshot
	if err := json.Unmarshal([]byte(reply.Output), &value, json.RejectUnknownMembers(true)); err != nil {
		return nil, err
	}
	return &value, nil
}

func liveScheduleRevision(t *testing.T, value framevalue.Number) uint64 {
	t.Helper()
	var revision uint64
	if err := framevalue.DecodeValue([]byte(value.String()), &revision, "schedule revision"); err != nil {
		t.Fatal(err)
	}
	return revision
}

func liveScheduleTime(t *testing.T, value string) time.Time {
	t.Helper()
	instant, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		t.Fatal(err)
	}
	return instant
}
