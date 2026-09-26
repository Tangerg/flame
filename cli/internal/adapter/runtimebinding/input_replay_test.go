package runtimebinding

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"errors"
	"iter"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	flameruntime "github.com/Tangerg/flame/runtime"
	"github.com/Tangerg/flame/runtime/protocol"

	"github.com/Tangerg/flame/cli/internal/adapter/filesystem/workbenchstate"
	"github.com/Tangerg/flame/cli/internal/application/agent/mutation"
	"github.com/Tangerg/flame/cli/internal/application/agent/workbench"
	"github.com/Tangerg/flame/cli/internal/domain/agent"
	"github.com/Tangerg/flame/cli/internal/domain/commandreplay"
)

// The binding stub records the actual protocol request after the production
// attachment reader and translator. Its first response models an accepted
// request whose acknowledgement was lost before local outbox retirement.
func TestMutationReplaysActualAttachmentBytesAfterWorkbenchRestart(t *testing.T) {
	for _, method := range []string{"start", "resume", "steer"} {
		for _, change := range []string{"changed", "deleted"} {
			t.Run(method+"/"+change, func(t *testing.T) {
				directory, source := t.TempDir(), t.TempDir()
				textPath, imagePath := filepath.Join(source, "notes.txt"), filepath.Join(source, "pixel.png")
				if err := os.WriteFile(textPath, []byte("original notes"), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(imagePath, []byte{0x89, 'P', 'N', 'G'}, 0o600); err != nil {
					t.Fatal(err)
				}
				message := agent.Message{Text: "inspect inputs", Attachments: []agent.Attachment{
					{ID: "notes", Kind: protocol.ContentBlockText, Name: "notes.txt", Path: textPath, MimeType: "text/plain", Size: 14},
					{ID: "pixel", Kind: protocol.ContentBlockImage, Name: "pixel.png", Path: imagePath, MimeType: "image/png", Size: 4},
				}}
				const commandID agent.CommandID = "cli_0123456789abcdef0123456789abcdef"
				var first []byte
				calls := 0
				capture := func(request any, key, namespace string) error {
					t.Helper()
					if key != string(commandID) || namespace != compatibleReplayNamespace {
						t.Fatalf("mutation changed identity: %q, %q", key, namespace)
					}
					body, err := json.Marshal(request, json.Deterministic(true))
					if err != nil {
						t.Fatal(err)
					}
					calls++
					if calls == 1 {
						first = body
						return context.DeadlineExceeded
					}
					if !bytes.Equal(first, body) {
						t.Fatalf("same identity replayed different protocol bytes:\nfirst %s\nagain %s", first, body)
					}
					return nil
				}
				stream := func(func(protocol.RunEvent, error) bool) {}
				binding := runBindingStub{
					start: func(_ context.Context, request protocol.StartRunRequest, options flameruntime.RunCommandOptions) (*protocol.StartRunResponse, iter.Seq2[protocol.RunEvent, error], error) {
						if err := capture(request, options.IdempotencyKey, options.IdempotencyNamespace); err != nil {
							return nil, nil, err
						}
						return &protocol.StartRunResponse{RunID: "run_1", SegmentID: "seg_1", UserItemID: "item_1"}, stream, nil
					},
					resume: func(_ context.Context, request protocol.ResumeRunRequest, options flameruntime.RunCommandOptions) (*protocol.ResumeRunResponse, iter.Seq2[protocol.RunEvent, error], error) {
						if err := capture(request, options.IdempotencyKey, options.IdempotencyNamespace); err != nil {
							return nil, nil, err
						}
						return &protocol.ResumeRunResponse{RunID: "run_1", SegmentID: "seg_2", UserItemID: new("item_2")}, stream, nil
					},
					steer: func(_ context.Context, request protocol.SteerRunRequest, options flameruntime.CommandOptions) (*protocol.SteerRunResponse, error) {
						if err := capture(request, options.IdempotencyKey, options.IdempotencyNamespace); err != nil {
							return nil, err
						}
						return &protocol.SteerRunResponse{UserItemID: "item_steer"}, nil
					},
				}
				connection := &Connection{runs: binding, loadAttachment: loadAttachmentFile, meta: requestMeta("test"),
					profile: profileWithFeatures(t, map[string]protocol.FeatureCapability{protocol.FeatureMultimodal: {Enabled: true}}),
				}
				store, err := workbenchstate.Open(directory)
				if err != nil {
					t.Fatal(err)
				}
				guard, err := commandreplay.NewProtectedGuard(compatibleReplayNamespace, time.Now().Add(time.Hour))
				if err != nil {
					t.Fatal(err)
				}
				start := agent.StartRun{CommandID: commandID, SessionID: "ses_1", Message: message,
					Options: agent.RunOptions{Provider: "mock", Model: "balanced"},
				}
				if method == "start" {
					if err := store.StagePendingRun(workbench.PendingRun{
						State: workbench.PendingRunQueued, Command: start,
						Replay: commandreplay.UnprotectedGuard(), CancelReplay: commandreplay.UnprotectedGuard(),
					}); err != nil {
						t.Fatal(err)
					}
					// Queuing does not read or freeze a file; edits remain effective
					// until this entry is actually admitted for its first dispatch.
					if err := os.WriteFile(textPath, []byte("latest queued notes"), 0o600); err != nil {
						t.Fatal(err)
					}
				}
				input, err := connection.PrepareInput(t.Context(), message)
				if err != nil {
					t.Fatal(err)
				}
				if method == "start" && !strings.Contains(input[1].Text, "latest queued notes") {
					t.Fatal("queue froze the source before first dispatch")
				}
				prepared, err := store.PrepareInput(t.Context(), message, input)
				if err != nil {
					t.Fatal(err)
				}
				switch method {
				case "start":
					err = store.MarkPendingRunDispatching(start.SessionID, commandID, guard, prepared)
				case "resume":
					approval := agent.Approval{RunID: "run_1", ItemID: "item_approval", Title: "Proceed?",
						Tool: &agent.ToolCall{Kind: agent.ToolShell, Name: "shell", Status: agent.ToolRunning}}
					err = store.StagePendingResume("ses_1", workbench.PendingResume{
						Command: agent.ResumeRun{CommandID: commandID, RunID: "run_1", Message: &message, Input: input,
							Answers: []agent.InterruptAnswer{{ItemID: approval.ItemID, Answer: agent.ApprovalAnswer{Decision: protocol.ApprovalDeny}}}},
						Interactions: []agent.Interaction{approval}, Replay: guard,
					}, prepared)
				case "steer":
					draft := agent.Message{Text: "/steer " + message.Text, Attachments: slices.Clone(message.Attachments)}
					if err := store.SaveDraft("ses_1", draft); err != nil {
						t.Fatal(err)
					}
					pending, createErr := workbench.NewPendingSteer("ses_1", agent.SteerRun{
						CommandID: commandID, RunID: "run_1", SegmentID: "seg_1", Message: message, Input: input,
					}, time.Now(), guard)
					if createErr != nil {
						t.Fatal(createErr)
					}
					err = store.StagePendingSteer(pending, draft, prepared)
				}
				if err != nil {
					t.Fatal(err)
				}
				dispatch := func(store *workbench.Store) error {
					t.Helper()
					switch method {
					case "start":
						command, err := store.PendingRuns("ses_1")[0].ReplayCommand()
						if err != nil {
							return err
						}
						_, err = connection.StartRun(t.Context(), command)
						return err
					case "resume":
						pending, _ := store.PendingResume("ses_1")
						command, err := pending.ReplayCommand()
						if err != nil {
							return err
						}
						_, err = connection.ResumeRun(t.Context(), command)
						return err
					default:
						pending, _ := store.PendingSteer("ses_1")
						command, err := pending.ReplayCommand()
						if err != nil {
							return err
						}
						_, err = connection.SteerRun(t.Context(), command)
						return err
					}
				}
				if err := dispatch(store); !errors.Is(err, context.DeadlineExceeded) || !mutation.OutcomeUnknown(err) {
					t.Fatalf("lost acknowledgement = %v", err)
				}
				if err := store.Close(); err != nil {
					t.Fatal(err)
				}
				for _, path := range []string{textPath, imagePath} {
					if change == "deleted" {
						err = os.Remove(path)
					} else {
						err = os.WriteFile(path, []byte("changed after commit"), 0o600)
					}
					if err != nil {
						t.Fatal(err)
					}
				}
				connection.loadAttachment = func(context.Context, string, int64) ([]byte, error) {
					t.Fatal("mutation replay reopened an original source path")
					return nil, nil
				}
				reopened, err := workbenchstate.Open(directory)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = reopened.Close() })
				if err := dispatch(reopened); err != nil {
					t.Fatal(err)
				}
				if calls != 2 {
					t.Fatalf("binding calls = %d, want 2", calls)
				}
			})
		}
	}
}
