//go:build darwin || linux

package execution_test

import (
	"bufio"
	"bytes"
	"context"
	json "encoding/json/v2"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"slices"
	"syscall"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	localruntime "github.com/Tangerg/flame/packages/local-runtime"
	"github.com/Tangerg/flame/runtime/internal/adapter/integration/pluginpackage"
	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/flame/runtime/internal/testsupport/persistencefixture"

	framecall "github.com/Tangerg/frame/call"
	"github.com/Tangerg/frame/process/runner"
	framevalue "github.com/Tangerg/frame/value"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/agent/interaction"
	scoperun "github.com/Tangerg/scope/agent/run"
)

const scheduleCrashKey = "schedule-crash-start"

// scheduleCut is one SIGKILL placement around the original create and the
// recovery outcome it must produce. Phases name the message-loss probe's stages.
type scheduleCut struct {
	name string
	// edited approves the create with an edited title before the first cut.
	edited bool
	// recutAt kills the recovering Runtime once more at that phase.
	recutAt testsupport.ScheduleCrashPhase
	// settled marks a recut that lands after Scope committed the resolution,
	// so recovery must not query the receipt again.
	settled bool
	// disables withdraws the schedules installation before recovery.
	disables bool
	// final is the phase recovery finishes under; an inconclusive one keeps
	// faulting the receipt it recovers through.
	final testsupport.ScheduleCrashPhase
}

var scheduleCuts = []scheduleCut{
	{name: "commit", final: testsupport.ScheduleCrashFinish},
	{name: "query", recutAt: testsupport.ScheduleCrashQuery, final: testsupport.ScheduleCrashFinish},
	{name: "resolution", recutAt: testsupport.ScheduleCrashResolution, settled: true, final: testsupport.ScheduleCrashFinish},
	{name: "edited", edited: true, final: testsupport.ScheduleCrashFinish},
	{name: "absent", final: testsupport.ScheduleCrashAbsent},
	{name: "conflict", final: testsupport.ScheduleCrashConflict},
	{name: "malformed", final: testsupport.ScheduleCrashMalformed},
	{name: "disabled", disables: true, final: testsupport.ScheduleCrashDisabled},
}

func (c scheduleCut) initialPhase() testsupport.ScheduleCrashPhase {
	if c.edited {
		return testsupport.ScheduleCrashEdited
	}
	return testsupport.ScheduleCrashCommit
}

func (c scheduleCut) inconclusive() bool {
	return c.final.Inconclusive()
}

func (c scheduleCut) title() string {
	if c.edited {
		return "edited title"
	}
	return "original title"
}

func TestScheduleCreateRecoversAcrossSIGKILLCuts(t *testing.T) {
	module, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	distribution := testsupport.TempDirectory(t)
	binary := filepath.Join(distribution, "bootstrap.test")
	build := exec.CommandContext(t.Context(), "go", "test", "-c", "-o", binary, "./internal/bootstrap")
	build.Dir = module
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build Runtime fixture: %v\n%s", err, output)
	}
	if err := testsupport.PrepareRuntimeCompanions(binary); err != nil {
		t.Fatalf("prepare Runtime companions: %v", err)
	}
	packages := filepath.Join(distribution, "plugins")
	build = exec.CommandContext(t.Context(), "node", filepath.Join(module, "../plugins/scripts/build-packages.mjs"), "--os", goruntime.GOOS, "--arch", goruntime.GOARCH, "--output", packages)
	build.Dir = module
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build real backends: %v\n%s", err, output)
	}

	for _, cut := range scheduleCuts {
		t.Run(cut.name, func(t *testing.T) {
			home := testsupport.TempDirectory(t)
			processRunner, err := runner.NewDaemon(runner.DaemonConfig{
				Executable: filepath.Join(distribution, "frame-runner"),
				Directory:  filepath.Join(home, "runner"), StartTimeout: 10 * time.Second,
			})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				if err := processRunner.Close(ctx); err != nil {
					t.Error(err)
				}
			})
			if _, err := processRunner.Identity(t.Context()); err != nil {
				t.Fatal(err)
			}
			source, evidence := scheduleLossPackage(t, module, packages)
			if err := syscall.Mkfifo(evidence+".ready", 0600); err != nil {
				t.Fatal(err)
			}
			ready, err := os.OpenFile(evidence+".ready", os.O_RDWR, 0600)
			if err != nil {
				t.Fatal(err)
			}
			defer ready.Close()

			helper := scheduleCrashHelper{
				binary: binary, home: home, source: source, evidence: evidence, packages: packages,
				edited: cut.edited, ready: ready,
			}
			initial := cut.initialPhase()
			helper.run(t, initial, true)
			stores, err := persistence.Open(t.Context(), persistence.Config{DataDirectory: home, ValidateDirectory: persistencefixture.UnleasedDirectory})
			if err != nil {
				t.Fatal(err)
			}
			receipt, err := stores.CommandAcceptances.Lookup(t.Context(), scheduleCrashKey)
			if err != nil || receipt.Use != runs.CommandKeyAccepted || receipt.CurrentRun == nil || receipt.CurrentRun.State() != run.Running {
				t.Fatalf("original opening: %+v,%v", receipt, err)
			}
			items, err := stores.Transcript.List(t.Context(), receipt.Acceptance.SessionID)
			if err != nil {
				t.Fatal(err)
			}
			var originalTool identity.ItemID
			var originalSource tool.Ref
			for _, created := range createToolItems(items) {
				originalTool, originalSource = created.item.ID(), created.invocation.Source
				server, remote, ok := originalSource.MCP()
				_, installed := server.Origin().Installation()
				if !ok || !installed || remote.String() != "create_schedule" {
					t.Fatalf("original Tool lost its installed source: %v", originalSource)
				}
				if created.item.Status() != transcript.ItemRunning {
					t.Fatalf("original Tool was already terminal: %+v", created.item)
				}
			}
			if originalTool.IsZero() {
				t.Fatal("original Tool Item was not committed")
			}
			scopeRoot := (scoperun.StartID{Scope: receipt.Acceptance.SessionID.String(), Key: receipt.Acceptance.RunID.String()}).Root()
			checkpoint, err := stores.ExecutorCheckpoints.LoadCheckpoint(t.Context(), scopeRoot.String())
			if err != nil {
				t.Fatalf("original Run's checkpoint: %v", err)
			}
			originalHead, found, err := stores.ExecutorCheckpoints.LoadExecutionTree(t.Context(), checkpoint.SessionID, checkpoint.RootMemberID)
			if err != nil || !found {
				t.Fatalf("original head: %t,%v", found, err)
			}
			originalTree, err := storedScopeLog(originalHead.Payload)
			if err != nil {
				t.Fatal(err)
			}
			var originalEffect scoperun.EffectID
			var originalEffects []scoperun.EffectID
			for _, intent := range createIntents(t, originalTree, originalTree.Root) {
				if intent.continuationOf != originalEffect {
					t.Fatal("original create call has multiple independent effect intents")
				}
				originalEffect = intent.id
				originalEffects = append(originalEffects, intent.id)
			}
			if !originalEffect.Valid() {
				t.Fatal("original create effect is unavailable")
			}
			if err := stores.Close(); err != nil {
				t.Fatal(err)
			}
			if cut.disables {
				helper.run(t, testsupport.ScheduleCrashDisable, false)
			}
			if cut.recutAt != "" {
				helper.run(t, cut.recutAt, true)
			}
			if cut.settled {
				assertScheduleResolutionCommitted(t, home, checkpoint.SessionID.String(), checkpoint.RootMemberID, originalEffect)
			}
			helper.run(t, cut.final, false)
			stores, err = persistence.Open(t.Context(), persistence.Config{DataDirectory: home, ValidateDirectory: persistencefixture.UnleasedDirectory})
			if err != nil {
				t.Fatal(err)
			}
			defer stores.Close()
			current, err := stores.CommandAcceptances.Lookup(t.Context(), scheduleCrashKey)
			if err != nil || !current.Acceptance.Equal(receipt.Acceptance) {
				t.Fatalf("recovery changed opening identity: %+v,%v", current, err)
			}
			steps := 3
			if cut.inconclusive() {
				steps = 2
			}
			if current.CurrentRun.Metrics().Steps() != steps {
				t.Fatalf("model steps=%d, want the original two calls and one continuation", current.CurrentRun.Metrics().Steps())
			}
			finalHead, found, err := stores.ExecutorCheckpoints.LoadExecutionTree(t.Context(), originalHead.SessionID, originalHead.RootID)
			if err != nil {
				t.Fatalf("final head: %t,%v", found, err)
			}
			if cut.disables {
				_, contextErr := stores.ExecutorCheckpoints.LoadCheckpoint(t.Context(), scopeRoot.String())
				if found || !errors.Is(contextErr, runs.ErrExecutorCheckpointNotFound) {
					t.Fatalf("unrecoverable execution retained a continuation: head=%t error=%v", found, contextErr)
				}
			} else {
				if !found {
					t.Fatal("recovered execution lost its original head")
				}
				finalTree, err := storedScopeLog(finalHead.Payload)
				if err != nil {
					t.Fatal(err)
				}
				var retained []scoperun.EffectID
				for _, intent := range createIntents(t, finalTree, originalTree.Root) {
					if !slices.Contains(originalEffects, intent.id) {
						t.Fatal("recovery prepared another create effect")
					}
					retained = append(retained, intent.id)
				}
				if !slices.Equal(retained, originalEffects) || finalTree.Root != originalTree.Root {
					t.Fatal("recovery replaced the original effect or process tree")
				}
			}
			items, err = stores.Transcript.List(t.Context(), receipt.Acceptance.SessionID)
			if err != nil {
				t.Fatal(err)
			}
			wantStatus := transcript.ItemCompleted
			if cut.inconclusive() {
				wantStatus = transcript.ItemIncomplete
			}
			recovered := createToolItems(items)
			for _, created := range recovered {
				if created.invocation.Source != originalSource {
					t.Fatalf("recovery replaced Tool source %v with %v", originalSource, created.invocation.Source)
				}
				if created.item.ID() != originalTool || created.item.Status() != wantStatus {
					t.Fatalf("recovery replaced the original Tool Item: %+v", created.item)
				}
			}
			if len(recovered) != 1 {
				t.Fatalf("canonical create Items=%d", len(recovered))
			}
			raw, err := os.ReadFile(evidence + "." + string(initial))
			if err != nil {
				t.Fatal(err)
			}
			var witness struct {
				DataDirectory string `json:"dataDirectory"`
				Request       struct {
					Params sdk.CallToolParamsRaw `json:"params"`
				} `json:"request"`
				Response struct {
					Result struct {
						StructuredContent struct {
							Schedule struct {
								ID string `json:"schedule_id"`
							} `json:"schedule"`
						} `json:"structuredContent"`
					} `json:"result"`
				} `json:"response"`
			}
			if err := json.Unmarshal(raw, &witness); err != nil {
				t.Fatal(err)
			}
			createdID := readScheduleOwner(t, packages, witness.DataDirectory, witness.Request.Params)
			if createdID != witness.Response.Result.StructuredContent.Schedule.ID {
				t.Fatalf("recovery changed the original creation: original ID=%q current ID=%q", witness.Response.Result.StructuredContent.Schedule.ID, createdID)
			}
			if cut.inconclusive() {
				if !cut.disables && len(current.CurrentRun.UnresolvedEffects()) != 1 {
					t.Fatalf("inconclusive proof lost its Unknown Effect: %+v", current.CurrentRun.UnresolvedEffects())
				}
				if _, err := os.Stat(filepath.Join(home, "recovered-model-result.json")); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("inconclusive evidence became a model result: %v", err)
				}
				return
			}
			result, err := os.ReadFile(filepath.Join(home, "recovered-model-result.json"))
			if err != nil {
				t.Fatal(err)
			}
			var view struct {
				Schedule struct {
					ID    string `json:"schedule_id"`
					Title string `json:"title"`
				} `json:"schedule"`
			}
			if err := json.Unmarshal(result, &view); err != nil || view.Schedule.Title != cut.title() || view.Schedule.ID == "" {
				t.Fatalf("original model result: %s,%v", result, err)
			}
			if createdID != view.Schedule.ID {
				t.Fatalf("model recovery changed the original result: owner ID=%q recovered ID=%q", createdID, view.Schedule.ID)
			}
			if cut.settled {
				if _, err := os.Stat(evidence + "." + string(testsupport.ScheduleCrashFinish)); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("a committed Scope settlement was queried again: %v", err)
				}
			}
		})
	}
}

// scheduleCrashHelper re-executes the Runtime crash helper for one phase. A
// killing run waits for the helper to report its durable cut on the ready
// FIFO and then SIGKILLs it; any other run must exit cleanly.
type scheduleCrashHelper struct {
	binary, home, source, evidence, packages string
	edited                                   bool
	ready                                    *os.File
}

func (h scheduleCrashHelper) run(t *testing.T, phase testsupport.ScheduleCrashPhase, kill bool) {
	t.Helper()
	if err := os.WriteFile(h.evidence+".phase", []byte(string(phase)), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 40*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, h.binary, "-test.run=^TestScheduleRecoveryCrashHelper$", "-test.timeout=35s")
	command.Env = append(os.Environ(), localruntime.ProductRootVariable+"="+h.home, "FLAME_TEST_SCHEDULE_CRASH="+string(phase), "FLAME_TEST_SCHEDULE_SOURCE="+h.source, "FLAME_TEST_SCHEDULE_EVIDENCE="+h.evidence, "FLAME_TEST_FIRST_PARTY_DIRECTORY="+h.packages)
	if h.edited {
		command.Env = append(command.Env, "FLAME_TEST_SCHEDULE_EDITED=1")
	}
	var output bytes.Buffer
	command.Stdout, command.Stderr = &output, &output
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	if !kill {
		if err := <-done; err != nil {
			t.Fatalf("fresh Runtime: %v\n%s", err, output.String())
		}
		return
	}
	h.awaitCut(t, ctx, phase, command, done, &output)
	if err := command.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	var exit *exec.ExitError
	if err := <-done; !errors.As(err, &exit) || exit.Sys().(syscall.WaitStatus).Signal() != syscall.SIGKILL {
		t.Fatalf("%s was not killed: %v", phase, err)
	}
}

// awaitCut waits until the helper reports that it reached its durable cut.
func (h scheduleCrashHelper) awaitCut(t *testing.T, ctx context.Context, phase testsupport.ScheduleCrashPhase, command *exec.Cmd, done <-chan error, output *bytes.Buffer) {
	t.Helper()
	barrier := make(chan error, 1)
	go func() {
		_, err := bufio.NewReader(h.ready).ReadString('\n')
		barrier <- err
	}()
	select {
	case err := <-barrier:
		if err != nil {
			t.Fatal(err)
		}
	case err := <-done:
		t.Fatalf("%s exited before its durable cut: %v\n%s", phase, err, output.String())
	case <-ctx.Done():
		_ = command.Process.Kill()
		<-done
		t.Fatalf("%s did not reach its durable cut\n%s", phase, output.String())
	}
}

func assertScheduleResolutionCommitted(t *testing.T, home, sessionID, rootID string, original scoperun.EffectID) {
	t.Helper()
	stores, err := persistence.Open(t.Context(), persistence.Config{DataDirectory: home, ValidateDirectory: persistencefixture.UnleasedDirectory})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := stores.Close(); err != nil {
			t.Error(err)
		}
	}()
	head, found, err := stores.ExecutorCheckpoints.LoadExecutionTree(t.Context(), testsupport.MustSessionID(sessionID), rootID)
	if err != nil || !found {
		t.Fatalf("resolution cut: found=%t error=%v", found, err)
	}
	tree, err := storedScopeLog(head.Payload)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range tree.Entries {
		settled, ok := entry.Record.(scoperun.EffectSettled)
		if ok && settled.ID == original && settled.Outcome.Status() == scoperun.SettlementStatusSucceeded {
			return
		}
	}
	t.Fatal("SIGKILL preceded the original create Effect's committed resolution")
}

func scheduleLossPackage(t *testing.T, module, packages string) (string, string) {
	t.Helper()
	source := filepath.Join(t.TempDir(), "schedules")
	if err := os.CopyFS(source, os.DirFS(filepath.Join(packages, "schedules"))); err != nil {
		t.Fatal(err)
	}
	evidence := filepath.Join(t.TempDir(), "cut")
	build := exec.CommandContext(t.Context(), "go", "build", "-trimpath", "-ldflags=-s -w", "-o", filepath.Join(source, "bin/message-loss"), "./testdata/managed-message-loss")
	build.Dir = filepath.Join(module, "internal/bootstrap")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build cut proxy: %v\n%s", err, output)
	}
	descriptor, err := json.Marshal(map[string]any{"$schema": "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json", "mcpServers": map[string]any{"schedules": map[string]any{"type": "stdio", "command": "./bin/message-loss", "args": []string{evidence, "./bin/schedules", "", "", "durable"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "mcp.json"), descriptor, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := pluginpackage.BuildRelease(t.Context(), source, pluginpackage.BuildTarget{OS: goruntime.GOOS, Arch: goruntime.GOARCH}, io.Discard); err != nil {
		t.Fatalf("build the actual message-loss release: %v", err)
	}
	return source, evidence
}

func readScheduleOwner(t *testing.T, packages, dataDirectory string, original sdk.CallToolParamsRaw) string {
	t.Helper()
	command := exec.CommandContext(t.Context(), filepath.Join(packages, "schedules", "bin", "schedules"))
	command.Env = append(os.Environ(), "PLUGIN_DATA="+dataDirectory)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	client := sdk.NewClient(&sdk.Implementation{Name: "schedule-owner-check"}, nil)
	if err := pluginmcp.RegisterClient(client); err != nil {
		t.Fatal(err)
	}
	session, err := client.Connect(t.Context(), &sdk.CommandTransport{Command: command}, nil)
	if err != nil {
		t.Fatalf("connect original backend: %v\n%s", err, stderr.String())
	}
	defer func() {
		if err := session.Close(); err != nil {
			t.Error(err)
		}
	}()
	contract, err := plugincontract.ParseCapability([]byte(testsupport.SchedulesCapabilityJSON))
	if err != nil {
		t.Fatal(err)
	}
	page, err := testsupport.CallCapability[testsupport.SchedulesListInput, testsupport.SchedulesPage](t.Context(), session, contract, "schedules", testsupport.SchedulesListV2Method, "", testsupport.SchedulesListInput{}, sdk.ParamsBase{})
	if err != nil || len(page.Rows) != 1 || page.NextCursor != "" {
		t.Fatalf("original backend creation: %+v, %v", page, err)
	}
	var invocation framecall.Invocation
	if err := json.Unmarshal(original.Meta[pluginmcp.InvocationMetadataKey], &invocation, json.RejectUnknownMembers(true)); err != nil {
		t.Fatal(err)
	}
	ctx, err := pluginmcp.WithInvocation(t.Context(), invocation)
	if err != nil {
		t.Fatal(err)
	}
	input, err := framevalue.ParseJSON(original.Arguments)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := pluginsdk.ReadToolReceipt(ctx, session, invocation.ID, original.Name, input)
	if err != nil || receipt == nil {
		t.Fatalf("committed original Tool receipt: %v, %v", receipt, err)
	}
	encoded, err := json.Marshal(receipt.Details)
	if err != nil {
		t.Fatal(err)
	}
	var output struct {
		Schedule struct {
			ID string `json:"schedule_id"`
		} `json:"schedule"`
	}
	if err := json.Unmarshal(encoded, &output); err != nil || output.Schedule.ID != page.Rows[0].Id {
		t.Fatalf("receipt differs from original creation: %s, %v", encoded, err)
	}
	return page.Rows[0].Id
}

type createToolItem struct {
	item       transcript.Item
	invocation transcript.ToolInvocation
}

// createToolItems lists the transcript's create_schedule Tool Items in order.
func createToolItems(items []transcript.Item) []createToolItem {
	var created []createToolItem
	for _, item := range items {
		if invocation, ok := testsupport.ToolInvocationOf(item); ok && invocation.Name == "schedules_create_schedule" {
			created = append(created, createToolItem{item: item, invocation: invocation})
		}
	}
	return created
}

type createIntent struct {
	id, continuationOf scoperun.EffectID
}

// createIntents lists the create_schedule effect intents the root process
// stepped, in log order.
func createIntents(t *testing.T, tree scoperun.Log, root scoperun.ProcessID) []createIntent {
	t.Helper()
	var intents []createIntent
	for _, entry := range tree.Entries {
		stepped, ok := entry.Record.(scoperun.Stepped)
		if !ok || stepped.Process != root {
			continue
		}
		for _, intent := range stepped.Intents() {
			call, tool, err := interaction.ToolCallFromEffect(intent.Effect)
			if err != nil {
				t.Fatal(err)
			}
			if tool && call.Call.Name == "schedules_create_schedule" {
				intents = append(intents, createIntent{id: intent.ID, continuationOf: call.ContinuationOf})
			}
		}
	}
	return intents
}

func storedScopeLog(payload []byte) (scoperun.Log, error) {
	var envelope struct {
		Log scoperun.Log `json:"log"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return scoperun.Log{}, err
	}
	return envelope.Log, envelope.Log.Validate()
}
