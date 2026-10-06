package runtime

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/feedback"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/flame/runtime/localruntime"
	"github.com/Tangerg/flame/runtime/protocol"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/history"
)

func TestExportTrajectoryReopensCompleteEvidenceThroughBinding(t *testing.T) {
	for _, name := range []string{
		"FLAME_PROVIDER", "FLAME_MODEL", "FLAME_APIKEY", "FLAME_BASEURL", "ANTHROPIC_API_KEY",
		"FLAME_MCP_SERVERS", "FLAME_A2A_AGENTS", "FLAME_A2A_RPC_ORIGINS",
	} {
		t.Setenv(name, "")
	}
	t.Setenv("FLAME_PROVIDER", "anthropic")
	cfg := Config{
		DataDirectory: t.TempDir(), UserHomePath: t.TempDir(), DefaultWorkspacePath: t.TempDir(),
		ConfigDirectories: []string{t.TempDir()},
	}
	rt, err := Open(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rt.Close() })
	ses, err := rt.CreateSession(t.Context(), protocol.CreateSessionRequest{}, CommandOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.CreateFeedback(t.Context(), protocol.FeedbackRequest{
		SessionID: ses.ID, Rating: protocol.FeedbackNegative, Text: "  misses the requested behavior  ",
	}, CommandOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := rt.Close(); err != nil {
		t.Fatal(err)
	}
	layout, err := localruntime.DataDirectoryAt(cfg.DataDirectory)
	if err != nil {
		t.Fatal(err)
	}
	db, err := sqlite.Open(t.Context(), layout.DatabasePath())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	at := time.Now().UTC().Add(-time.Hour).Truncate(time.Millisecond)
	root := testsupport.MustRestoreRun(run.Snapshot{
		ID: "run_export_root", SessionID: ses.ID, State: run.Completed,
		Capabilities: run.Capabilities{ChildRuns: true}, CreatedAt: at, FinishedAt: at.Add(time.Minute),
	})
	child := testsupport.MustRestoreRun(run.Snapshot{
		ID: "run_export_child", SessionID: ses.ID, State: run.Canceled,
		Capabilities: root.Capabilities(), CreatedAt: at, FinishedAt: at.Add(time.Minute),
		Lineage: run.Lineage{SpawnedByItemID: "item_export_spawn", ParentRunID: root.ID(), RootRunID: root.ID()},
	})
	for _, value := range []run.Run{root, child} {
		if err := sqlite.NewRunStore(db).Restore(t.Context(), value); err != nil {
			t.Fatal(err)
		}
	}
	for _, value := range []transcript.Item{
		testsupport.MustRestoreItem(testsupport.ItemInput{
			ID: "item_export_spawn", SessionID: ses.ID, RunID: root.ID(), Kind: transcript.ToolCall,
			Status: transcript.ItemIncomplete, Tool: &transcript.ToolInvocation{Name: "delegate_task"}, OccurredAt: at,
		}),
		testsupport.MustRestoreItem(testsupport.ItemInput{
			ID: "item_export_child", SessionID: ses.ID, RunID: child.ID(), Kind: transcript.AgentMessage,
			Content: []transcript.ContentBlock{{Kind: transcript.TextContent, Text: "child evidence"}}, OccurredAt: at,
		}),
	} {
		if err := sqlite.NewTranscriptStore(db).AppendItem(t.Context(), value); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := sqlite.NewMessageStore(db).Write(t.Context(), history.ConversationID(ses.ID),
		chat.NewUserMessage(chat.NewTextPart("retained question")),
	); err != nil {
		t.Fatal(err)
	}
	for index := range 102 {
		runID, state := root.ID(), "completed"
		var usage any
		if index == 0 {
			usage = `{"inputTokens":0,"outputTokens":0,"cacheReadTokens":null,"cacheWriteTokens":null,"reasoningTokens":null}`
		}
		if index == 101 {
			runID, state = child.ID(), "unknown"
		}
		if _, err := db.ExecContext(t.Context(), `INSERT INTO model_invocations
			(call_id,session_id,run_id,segment_id,state,started_at,finished_at,usage) VALUES(?,?,?,?,?,?,?,?)`,
			fmt.Sprintf("model:export:%03d", index), ses.ID, runID, "seg_export", state,
			at.Add(time.Duration(index)*time.Millisecond).UnixNano(), at.Add(time.Second).UnixNano(), usage,
		); err != nil {
			t.Fatal(err)
		}
	}
	for index, state := range []string{"incomplete", "completed"} {
		if _, err := db.ExecContext(t.Context(), `INSERT INTO tool_invocations
			(call_id,item_id,session_id,run_id,segment_id,state,started_at,finished_at) VALUES(?,?,?,?,?,?,?,?)`,
			"tool:export:1", "item_export_spawn", ses.ID, root.ID(), fmt.Sprintf("seg_tool_%d", index), state,
			at.Add(time.Duration(index)*time.Second).UnixNano(), at.Add(time.Duration(index+1)*time.Second).UnixNano(),
		); err != nil {
			t.Fatal(err)
		}
	}
	for _, entry := range []feedback.Entry{
		{RunID: root.ID(), Text: "run feedback", CreatedAt: at},
		{ItemID: "item_export_child", Text: "item feedback", CreatedAt: at},
		{SessionID: ses.ID, RunID: "run_already_removed", Text: "retained orphan feedback", CreatedAt: at},
		{Text: "general feedback", CreatedAt: at},
	} {
		if err := sqlite.NewFeedbackStore(db).Append(t.Context(), entry); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	rt, err = Open(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	request := protocol.ExportTrajectoryRequest{SessionID: ses.ID}
	if response, err := rt.ExportTrajectory(t.Context(), request, CallOptions{}); !errors.Is(err, protocol.ErrCapabilityNotNeg) || response != nil {
		t.Fatalf("child evidence without capability = %v, %v", response, err)
	}
	options := CallOptions{RequestMeta: protocol.RequestMeta{ClientCapabilities: &protocol.ClientCapabilities{
		Features: map[string]protocol.FeaturePreference{protocol.FeatureSubagents: {Enabled: true}},
	}}}
	page, err := rt.ListSessionTrajectory(t.Context(), protocol.ListSessionTrajectoryRequest{SessionID: ses.ID}, options)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(page.Data, func(entry protocol.TrajectoryEntry) bool {
		return entry.Model != nil && entry.Model.CallID == "model:export:100"
	}) {
		t.Fatal("timeline lost the exact persisted model identity")
	}
	calls, err := rt.ListModelInvocations(t.Context(), protocol.ListModelInvocationsRequest{RunID: root.ID()}, options)
	if err != nil || len(calls.Data) == 0 || calls.Data[0].CallID != "model:export:100" {
		t.Fatalf("model invocation page = %+v, %v", calls, err)
	}
	exported, err := rt.ExportTrajectory(t.Context(), request, options)
	if err != nil {
		t.Fatal(err)
	}
	trajectory := exported.Trajectory
	if trajectory.SchemaVersion != protocol.SessionTrajectoryVersion || trajectory.Session.ID != ses.ID ||
		len(trajectory.Runs) != 2 || len(trajectory.Items) != 2 || len(trajectory.ModelInvocations) != 102 ||
		len(trajectory.ToolAttempts) != 2 || len(trajectory.Messages) != 1 || len(trajectory.Feedback) != 4 {
		t.Fatalf("incomplete exported evidence: runs=%d items=%d models=%d tools=%d messages=%d feedback=%d",
			len(trajectory.Runs), len(trajectory.Items), len(trajectory.ModelInvocations), len(trajectory.ToolAttempts), len(trajectory.Messages), len(trajectory.Feedback))
	}
	if trajectory.ModelInvocations[0].CallID != "model:export:000" || trajectory.ToolAttempts[0].CallID != "tool:export:1" {
		t.Fatal("export changed persisted execution identities")
	}
	if trajectory.ModelInvocations[0].Usage == nil || trajectory.ModelInvocations[1].Usage != nil ||
		trajectory.ModelInvocations[101].State != protocol.ModelInvocationUnknown {
		t.Fatal("export lost absent, reported-zero, or unknown invocation evidence")
	}
	if trajectory.ToolAttempts[0].SegmentID == trajectory.ToolAttempts[1].SegmentID ||
		trajectory.ToolAttempts[0].State != protocol.ToolAttemptIncomplete || trajectory.ToolAttempts[1].State != protocol.ToolAttemptCompleted {
		t.Fatalf("Tool attempt history = %+v", trajectory.ToolAttempts)
	}
	if !slices.ContainsFunc(trajectory.Feedback, func(entry protocol.FeedbackEntry) bool {
		return entry.Text == "  misses the requested behavior  " && entry.Rating == protocol.FeedbackNegative
	}) {
		t.Fatal("export rewrote the user's independent quality signal")
	}
	for _, entry := range trajectory.Feedback {
		if entry.CreatedAt.After(trajectory.CollectedAt) {
			t.Fatal("export collection timestamp precedes included feedback")
		}
	}
	encoded, err := json.Marshal(trajectory)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]jsontext.Value
	if err := json.Unmarshal(encoded, &document); err != nil || len(document["limitations"]) == 0 {
		t.Fatalf("structured evidence document = %v", err)
	}
	if _, importable := document["version"]; importable {
		t.Fatal("evaluation document masquerades as an importable Session artifact")
	}
}
