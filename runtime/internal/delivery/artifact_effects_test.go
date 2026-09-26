package delivery

import (
	json "encoding/json/v2"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/application/agent/sessions"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/flame/runtime/protocol"
)

func TestArtifactOutcomePreservesUnresolvedEffects(t *testing.T) {
	effect, err := run.NewUnresolvedEffect("process_source", "effect_source", "canceled", "owner stopped", "external result is unconfirmed")
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		outcome run.Outcome
		failure *run.Failure
	}{
		{outcome: run.OutcomeCompleted},
		{outcome: run.OutcomeCanceled},
		{outcome: run.OutcomeTimedOut, failure: &run.Failure{Kind: run.FailureTimeout}},
		{outcome: run.OutcomeFailed, failure: &run.Failure{Kind: run.FailureProviderUnavailable}},
		{outcome: run.OutcomeLost, failure: &run.Failure{Kind: run.FailureLost}},
	} {
		t.Run(string(test.outcome), func(t *testing.T) {
			portable := sessions.PortableRun{
				ID: "run_source", SessionID: "ses_source", Selection: testsupport.DefaultModelSelection(),
				Outcome: test.outcome, Failure: test.failure, UnresolvedEffects: []run.UnresolvedEffect{effect},
			}
			artifact, err := artifactRunFromPortable(portable)
			if err != nil {
				t.Fatalf("encode: %v", err)
			}
			if err := protocol.ValidateWireTree(artifact.Outcome); err != nil {
				t.Fatalf("validate outcome: %v", err)
			}
			want := []protocol.UnresolvedEffect{{
				ProcessID: "process_source", EffectID: "effect_source", Cause: "canceled",
				Reason: "owner stopped", Detail: "external result is unconfirmed",
			}}
			if !slices.Equal(artifact.Outcome.UnresolvedEffects, want) {
				t.Fatalf("artifact effects = %+v, want %+v", artifact.Outcome.UnresolvedEffects, want)
			}
			decoded, err := portableRunFromArtifact("artifact.runs[0]", artifact)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			artifact.Outcome.UnresolvedEffects[0].Detail = "changed artifact"
			if decoded.Outcome != test.outcome || !slices.Equal(decoded.UnresolvedEffects, []run.UnresolvedEffect{effect}) {
				t.Fatalf("decoded terminal facts = %+v, want original outcome and evidence", decoded)
			}
		})
	}
}

func TestArtifactOutcomeOmitsEmptyUnresolvedEffects(t *testing.T) {
	for _, effects := range [][]run.UnresolvedEffect{nil, {}} {
		artifact, err := artifactRunFromPortable(sessions.PortableRun{Outcome: run.OutcomeCanceled, UnresolvedEffects: effects})
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(artifact.Outcome)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(encoded), "unresolvedEffects") {
			t.Fatalf("empty evidence was encoded: %s", encoded)
		}
	}
}

func TestSessionExportImportPreservesUnresolvedEffectsAsHistory(t *testing.T) {
	source, sourceRuntime := rollbackHarness(t)
	destination, destinationRuntime := rollbackHarness(t)
	ctx := WithRequestMeta(t.Context(), protocol.RequestMeta{ClientCapabilities: &protocol.ClientCapabilities{
		Features: map[string]protocol.FeaturePreference{protocol.FeatureSubagents: {Enabled: true}},
	}})
	ses, err := insertSessionFixture(ctx, sourceRuntime.sess, "Unconfirmed external results", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	rootEffect, err := run.NewUnresolvedEffect("process_root_source", "effect_root_source", "failed", "delegate unresolved", "root observation")
	if err != nil {
		t.Fatal(err)
	}
	childEffect, err := run.NewUnresolvedEffect("process_child_source", "effect_child_source", "canceled", "owner stopped", "child observation")
	if err != nil {
		t.Fatal(err)
	}
	rootOutcome, childOutcome := run.OutcomeLost, run.OutcomeCanceled
	root := testsupport.MustRestoreRun(run.Snapshot{
		SessionID: ses.ID(), ID: "run_root_source", Outcome: &rootOutcome,
		Capabilities: run.Capabilities{ChildRuns: true}, UnresolvedEffects: []run.UnresolvedEffect{rootEffect},
	})
	child := testsupport.MustRestoreRun(run.Snapshot{
		SessionID: ses.ID(), ID: "run_child_source", Outcome: &childOutcome,
		Capabilities: root.Capabilities(), UnresolvedEffects: []run.UnresolvedEffect{childEffect},
		Lineage: run.Lineage{SpawnedByItemID: "item_spawn_source", ParentRunID: root.ID(), RootRunID: root.ID()},
	})
	if err := sourceRuntime.runs.Restore(ctx, root); err != nil {
		t.Fatalf("seed root: %v", err)
	}
	if err := sourceRuntime.hist.AppendItem(ctx, testsupport.MustRestoreItem(testsupport.ItemInput{
		SessionID: ses.ID(), RunID: root.ID(), ID: "item_spawn_source",
		Status: transcript.ItemIncomplete, Kind: transcript.ToolCall,
		Tool: &transcript.ToolInvocation{Name: "delegate_task"},
	})); err != nil {
		t.Fatalf("seed delegate Item: %v", err)
	}
	if err := sourceRuntime.runs.Restore(ctx, child); err != nil {
		t.Fatalf("seed child: %v", err)
	}

	exported, err := source.ExportSession(ctx, protocol.ExportSessionRequest{SessionID: ses.ID()})
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	encoded, err := json.Marshal(exported.Artifact)
	if err != nil {
		t.Fatal(err)
	}
	var artifact protocol.SessionArtifact
	if err := json.Unmarshal(encoded, &artifact, json.RejectUnknownMembers(true)); err != nil {
		t.Fatalf("decode JSON artifact: %v", err)
	}
	if _, err := destination.ImportSession(ctx, protocol.ImportSessionRequest{Artifact: artifact}); err != nil {
		t.Fatalf("import: %v", err)
	}
	for _, original := range []run.Run{root, child} {
		restored, found, err := destinationRuntime.runs.Run(ctx, original.ID())
		if err != nil || !found {
			t.Fatalf("read imported Run %q: found %v, error %v", original.ID(), found, err)
		}
		if !restored.Equal(original) {
			t.Fatalf("imported Run %q = %+v, want %+v", original.ID(), restored.Snapshot(), original.Snapshot())
		}
		view, err := destination.GetRun(ctx, protocol.GetRunRequest{RunID: original.ID()})
		if err != nil {
			t.Fatalf("read imported Run view: %v", err)
		}
		if view.Status != protocol.RunStatusFinished || view.ActiveSegmentID != "" || view.Outcome == nil ||
			!slices.Equal(view.Outcome.UnresolvedEffects, presentUnresolvedEffects(original.UnresolvedEffects())) {
			t.Fatalf("imported Run view = %+v, want terminal historical evidence", view)
		}
		if _, _, err := destination.ResumeRun(ctx, protocol.ResumeRunRequest{RunID: original.ID()}); !errors.Is(err, protocol.ErrInterruptNotOpen) {
			t.Fatalf("resume imported Run %q error = %v, want interrupt_not_open", original.ID(), err)
		}
	}
	open, err := destinationRuntime.interrupts.List(ctx, ses.ID())
	if err != nil || len(open) != 0 {
		t.Fatalf("imported open interrupts = %+v, error %v, want none", open, err)
	}
	reexported, err := destination.ExportSession(ctx, protocol.ExportSessionRequest{SessionID: ses.ID()})
	if err != nil {
		t.Fatalf("re-export: %v", err)
	}
	expected := artifact
	expected.Session.Workspace.Path = canonicalWorkspacePath(t, expected.Session.Workspace.Path)
	if before, after := encodeArtifact(t, expected), encodeArtifact(t, *reexported.Artifact); before != after {
		t.Fatalf("historical artifact changed after import\n before: %s\n after: %s", before, after)
	}
}

func TestSessionImportRejectsInvalidUnresolvedEffects(t *testing.T) {
	effect := protocol.UnresolvedEffect{ProcessID: "process_source", EffectID: "effect_source", Cause: "canceled"}
	for name, effects := range map[string][]protocol.UnresolvedEffect{
		"empty identity":        {{ProcessID: "process_source", Cause: "canceled"}},
		"noncanonical identity": {{ProcessID: " process_source", EffectID: "effect_source", Cause: "canceled"}},
		"oversized diagnostic":  {{ProcessID: "process_source", EffectID: "effect_source", Cause: "canceled", Detail: strings.Repeat("x", 4097)}},
		"duplicate effect":      {effect, effect},
	} {
		t.Run(name, func(t *testing.T) {
			handler, rt := rollbackHarness(t)
			at := time.Unix(1, 0).UTC()
			artifact := protocol.SessionArtifact{
				Version: protocol.SessionArtifactVersion,
				Session: protocol.ArtifactSession{
					ID: "ses_source", Title: "Unconfirmed", Workspace: protocol.WorkspaceRef{Path: t.TempDir()},
					Provider: "test-provider", Model: "test-model", CreatedAt: at, UpdatedAt: at,
				},
				Runs: []protocol.ArtifactRun{{
					ID: "run_source", SessionID: "ses_source", Provider: "test-provider", Model: "test-model",
					ProtocolProfile: &protocol.RunProtocolProfile{}, CreatedAt: at, FinishedAt: at, UpdatedAt: at,
					Outcome: protocol.ArtifactOutcome{Type: protocol.ArtifactOutcomeCanceled, UnresolvedEffects: effects},
				}},
			}
			if _, err := handler.ImportSession(t.Context(), protocol.ImportSessionRequest{Artifact: artifact}); !errors.Is(err, protocol.ErrInvalidParams) || !strings.Contains(strings.ToLower(err.Error()), "unresolved") {
				t.Fatalf("import error = %v, want invalid unresolved effect rejection", err)
			}
			if _, err := rt.sess.Get(t.Context(), artifact.Session.ID); !errors.Is(err, session.ErrNotFound) {
				t.Fatalf("invalid evidence import wrote the Session: %v", err)
			}
		})
	}
}
