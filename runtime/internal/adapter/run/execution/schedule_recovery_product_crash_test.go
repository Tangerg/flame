//go:build darwin || linux

package execution_test

import (
	"bufio"
	"bytes"
	"context"
	json "encoding/json/v2"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"syscall"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/jsonvalue"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	sdk "github.com/Tangerg/go-sdk/mcp"
	agent "github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/agent/strategy/interaction"
)

const scheduleCrashKey = "schedule-crash-start"

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
	packages := filepath.Join(distribution, "plugins")
	build = exec.CommandContext(t.Context(), "node", filepath.Join(module, "../plugins/scripts/build-packages.mjs"), "--os", goruntime.GOOS, "--arch", goruntime.GOARCH, "--output", packages)
	build.Dir = module
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build real backends: %v\n%s", err, output)
	}

	for _, cut := range []string{"commit", "query", "resolution", "edited", "absent", "conflict", "malformed", "disabled"} {
		t.Run(cut, func(t *testing.T) {
			home := testsupport.TempDirectory(t)
			source, evidence := scheduleLossPackage(t, module, packages)
			if err := syscall.Mkfifo(evidence+".ready", 0600); err != nil {
				t.Fatal(err)
			}
			ready, err := os.OpenFile(evidence+".ready", os.O_RDWR, 0600)
			if err != nil {
				t.Fatal(err)
			}
			defer ready.Close()
			if err := syscall.Mkfifo(evidence+".continue", 0600); err != nil {
				t.Fatal(err)
			}
			permit, err := os.OpenFile(evidence+".continue", os.O_RDWR, 0600)
			if err != nil {
				t.Fatal(err)
			}
			defer permit.Close()
			invoke := func(phase string, kill bool) {
				t.Helper()
				if err := os.WriteFile(evidence+".phase", []byte(phase), 0600); err != nil {
					t.Fatal(err)
				}
				ctx, cancel := context.WithTimeout(t.Context(), 40*time.Second)
				defer cancel()
				command := exec.CommandContext(ctx, binary, "-test.run=^TestScheduleRecoveryCrashHelper$", "-test.timeout=35s")
				command.Env = append(os.Environ(), "FLAME_HOME="+home, "FLAME_TEST_SCHEDULE_CRASH="+phase, "FLAME_TEST_SCHEDULE_SOURCE="+source, "FLAME_TEST_SCHEDULE_EVIDENCE="+evidence, "FLAME_TEST_FIRST_PARTY_DIRECTORY="+packages)
				if cut == "edited" {
					command.Env = append(command.Env, "FLAME_TEST_SCHEDULE_EDITED=1")
				}
				var output bytes.Buffer
				command.Stdout, command.Stderr = &output, &output
				if err := command.Start(); err != nil {
					t.Fatal(err)
				}
				done := make(chan error, 1)
				go func() { done <- command.Wait() }()
				if kill {
					barrier := make(chan error, 1)
					go func() {
						reader := bufio.NewReader(ready)
						for {
							if _, err := reader.ReadString('\n'); err != nil {
								barrier <- err
								return
							}
							if phase == "resolution" {
								settled, err := scheduleResolutionCommitted(ctx, home)
								if err != nil {
									barrier <- err
									return
								}
								if !settled {
									if _, err := permit.Write([]byte{1}); err != nil {
										barrier <- err
										return
									}
									continue
								}
							}
							barrier <- nil
							return
						}
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
					if err := command.Process.Kill(); err != nil {
						t.Fatal(err)
					}
					var exit *exec.ExitError
					if err := <-done; !errors.As(err, &exit) || exit.Sys().(syscall.WaitStatus).Signal() != syscall.SIGKILL {
						t.Fatalf("%s was not killed: %v", phase, err)
					}
				} else if err := <-done; err != nil {
					t.Fatalf("fresh Runtime: %v\n%s", err, output.String())
				}
			}
			initial := "commit"
			if cut == "edited" {
				initial = "edited"
			}
			invoke(initial, true)
			stores, err := persistence.Open(t.Context(), persistence.Config{DataDirectory: home})
			if err != nil {
				t.Fatal(err)
			}
			receipt, err := stores.CommandAcceptances.Lookup(t.Context(), scheduleCrashKey)
			if err != nil || !receipt.Found || receipt.CurrentRun == nil || receipt.CurrentRun.State() != run.Running {
				t.Fatalf("original opening: %+v,%v", receipt, err)
			}
			items, err := stores.Transcript.List(t.Context(), receipt.Acceptance.SessionID)
			if err != nil {
				t.Fatal(err)
			}
			originalTool := ""
			for _, item := range items {
				if invocation, ok := item.ToolInvocation(); ok && invocation.Name == "schedules_create_schedule" {
					originalTool = item.ID()
					if item.Status() != transcript.ItemRunning {
						t.Fatalf("original Tool was already terminal: %+v", item)
					}
				}
			}
			if originalTool == "" {
				t.Fatal("original Tool Item was not committed")
			}
			checkpoint, found, err := stores.ExecutorCheckpoints.LoadSessionCheckpoint(t.Context(), receipt.Acceptance.SessionID)
			if err != nil || !found {
				t.Fatalf("original context: %t,%v", found, err)
			}
			originalHead, found, err := stores.ExecutorCheckpoints.LoadExecutionTree(t.Context(), checkpoint.SessionID, checkpoint.RootMemberID)
			if err != nil || !found {
				t.Fatalf("original head: %t,%v", found, err)
			}
			originalTree, err := agent.ParseTreeSnapshot(originalHead.Payload)
			if err != nil {
				t.Fatal(err)
			}
			var originalChild agent.ProcessSnapshot
			for _, child := range originalTree.ProcessSnapshots() {
				if !child.Relation().IsRoot() && !child.Status().Terminal() {
					originalChild = child
				}
			}
			if !originalChild.Valid() {
				t.Fatal("original create Process is unavailable")
			}
			if cut == "disabled" {
				for _, dependency := range checkpoint.Installations {
					snapshot, found, err := stores.Installations.Get(t.Context(), dependency.InstallationID)
					if err != nil || !found {
						t.Fatalf("original installation: %t,%v", found, err)
					}
					snapshot.Installation.Disable()
					if err := stores.Installations.Save(t.Context(), snapshot.Installation); err != nil {
						t.Fatal(err)
					}
				}
			}
			if err := stores.Close(); err != nil {
				t.Fatal(err)
			}
			negative := cut == "absent" || cut == "conflict" || cut == "malformed" || cut == "disabled"
			if cut == "query" || cut == "resolution" {
				invoke(cut, true)
			}
			phase := "finish"
			if negative {
				phase = cut
			}
			invoke(phase, false)
			stores, err = persistence.Open(t.Context(), persistence.Config{DataDirectory: home})
			if err != nil {
				t.Fatal(err)
			}
			defer stores.Close()
			current, err := stores.CommandAcceptances.Lookup(t.Context(), scheduleCrashKey)
			if err != nil || !current.Acceptance.Equal(receipt.Acceptance) {
				t.Fatalf("recovery changed opening identity: %+v,%v", current, err)
			}
			steps := 3
			if negative {
				steps = 2
			}
			if current.CurrentRun.Metrics().Steps() != steps {
				t.Fatalf("model steps=%d, want the original two calls and one continuation", current.CurrentRun.Metrics().Steps())
			}
			finalHead, found, err := stores.ExecutorCheckpoints.LoadExecutionTree(t.Context(), originalHead.SessionID, originalHead.RootID)
			if err != nil {
				t.Fatalf("final head: %t,%v", found, err)
			}
			if cut == "disabled" {
				_, contextFound, contextErr := stores.ExecutorCheckpoints.LoadSessionCheckpoint(t.Context(), originalHead.SessionID)
				if found || contextFound || contextErr != nil {
					t.Fatalf("unrecoverable execution retained a continuation: head=%t context=%t error=%v", found, contextFound, contextErr)
				}
			} else {
				if !found {
					t.Fatal("recovered execution lost its original head")
				}
				finalTree, err := agent.ParseTreeSnapshot(finalHead.Payload)
				if err != nil {
					t.Fatal(err)
				}
				retained := false
				for _, child := range finalTree.ProcessSnapshots() {
					if child.Relation().ProcessID() == originalChild.Relation().ProcessID() {
						retained = true
						if child.Usage().PreparedEffects != originalChild.Usage().PreparedEffects {
							t.Fatal("recovery prepared another create Effect")
						}
					}
				}
				if !retained || finalTree.RootID() != originalTree.RootID() {
					t.Fatal("recovery replaced the original Process tree")
				}
			}
			items, err = stores.Transcript.List(t.Context(), receipt.Acceptance.SessionID)
			if err != nil {
				t.Fatal(err)
			}
			count := 0
			wantStatus := transcript.ItemCompleted
			if negative {
				wantStatus = transcript.ItemIncomplete
			}
			for _, item := range items {
				if invocation, ok := item.ToolInvocation(); ok && invocation.Name == "schedules_create_schedule" {
					count++
					if item.ID() != originalTool || item.Status() != wantStatus {
						t.Fatalf("recovery replaced the original Tool Item: %+v", item)
					}
				}
			}
			if count != 1 {
				t.Fatalf("canonical create Items=%d", count)
			}
			raw, err := os.ReadFile(evidence + "." + initial)
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
			if negative {
				if cut != "disabled" && len(current.CurrentRun.UnresolvedEffects()) != 1 {
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
			title := "original title"
			if cut == "edited" {
				title = "edited title"
			}
			if err := json.Unmarshal(result, &view); err != nil || view.Schedule.Title != title || view.Schedule.ID == "" {
				t.Fatalf("original model result: %s,%v", result, err)
			}
			if createdID != view.Schedule.ID {
				t.Fatalf("model recovery changed the original result: owner ID=%q recovered ID=%q", createdID, view.Schedule.ID)
			}
			if cut == "resolution" {
				if _, err := os.Stat(evidence + ".finish"); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("a committed Scope settlement was queried again: %v", err)
				}
			}
		})
	}
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
	return source, evidence
}

func scheduleResolutionCommitted(ctx context.Context, home string) (settled bool, resultErr error) {
	stores, err := persistence.Open(ctx, persistence.Config{DataDirectory: home})
	if err != nil {
		return false, err
	}
	defer func() { resultErr = errors.Join(resultErr, stores.Close()) }()
	receipt, err := stores.CommandAcceptances.Lookup(ctx, scheduleCrashKey)
	if err != nil || !receipt.Found {
		return false, err
	}
	checkpoint, found, err := stores.ExecutorCheckpoints.LoadSessionCheckpoint(ctx, receipt.Acceptance.SessionID)
	if err != nil || !found {
		return false, err
	}
	head, found, err := stores.ExecutorCheckpoints.LoadExecutionTree(ctx, checkpoint.SessionID, checkpoint.RootMemberID)
	if err != nil || !found {
		return false, err
	}
	tree, err := agent.ParseTreeSnapshot(head.Payload)
	if err != nil {
		return false, err
	}
	for _, process := range tree.ProcessSnapshots() {
		for id, settlement := range process.Settlements() {
			if settlement.Status() != agent.SettlementStatusSucceeded {
				continue
			}
			request, found := tree.EffectRequest(process.Relation().ProcessID(), id)
			if !found {
				continue
			}
			invocation, err := interaction.ToolInvocationFromRequest(request)
			if err == nil && invocation.ToolCall().Name == "schedules_create_schedule" {
				return true, nil
			}
		}
	}
	return false, nil
}

// Inspect a stopped release through the public backend contract. The crash
// driver never reads or edits the plugin's private schema or ledger.
func readScheduleOwner(t *testing.T, packages, dataDirectory string, original sdk.CallToolParamsRaw) string {
	t.Helper()
	command := exec.CommandContext(t.Context(), filepath.Join(packages, "schedules", "bin", "schedules"))
	command.Env = append(os.Environ(), "PLUGIN_DATA="+dataDirectory)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	client := sdk.NewClient(&sdk.Implementation{Name: "schedule-owner-check"}, nil)
	if err := pluginsdk.RegisterClient(client); err != nil {
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
	contract, err := extension.ParseCapability([]byte(testsupport.SchedulesCapabilityJSON))
	if err != nil {
		t.Fatal(err)
	}
	page, err := testsupport.CallCapability[testsupport.SchedulesListV1Input, testsupport.SchedulesListV1Output](t.Context(), session, contract, "schedules", testsupport.SchedulesListV1Method, "", testsupport.SchedulesListV1Input{}, sdk.ParamsBase{})
	if err != nil || len(page.Rows) != 1 || page.NextCursor != "" {
		t.Fatalf("original backend creation: %+v, %v", page, err)
	}
	var invocation extension.Invocation
	if err := json.Unmarshal(original.Meta[pluginsdk.InvocationMetadataKey], &invocation, json.RejectUnknownMembers(true)); err != nil {
		t.Fatal(err)
	}
	ctx, err := pluginsdk.WithInvocation(t.Context(), invocation)
	if err != nil {
		t.Fatal(err)
	}
	input, err := jsonvalue.Parse(original.Arguments)
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
	if err := json.Unmarshal(encoded, &output); err != nil || output.Schedule.ID != page.Rows[0].ID {
		t.Fatalf("receipt differs from original creation: %s, %v", encoded, err)
	}
	return page.Rows[0].ID
}
