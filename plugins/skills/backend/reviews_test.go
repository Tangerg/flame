package main

import (
	json "encoding/json/v2"
	"path/filepath"
	"testing"

	"github.com/Tangerg/frame/portable"
	framemcp "github.com/Tangerg/frame/process/mcp"
	framevalue "github.com/Tangerg/frame/value"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/skilllearning"
	sdk "github.com/Tangerg/go-sdk/mcp"

	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
)

func reviewCall[P, R any](t *testing.T, session *sdk.ClientSession, method *pluginmcp.Method[P, R], id string, origin framemcp.Origin, p P) (R, error) {
	t.Helper()
	var zero R
	capability, err := skilllearning.Contract()
	if err != nil {
		t.Fatal(err)
	}
	export, err := pluginmcp.ResolveCapability(t.Context(), session, "skill-learning", capability)
	if err != nil {
		t.Fatal(err)
	}
	bound, err := export.Method(method.Name())
	if err != nil {
		t.Fatal(err)
	}
	input, err := framevalue.EncodeJSON(p)
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := pluginmcp.WithInvocation(t.Context(), framemcp.Invocation{ID: id, Origin: origin, Target: portable.Target{Scope: extension.ScopeInstallation, Reference: "skills"}})
	if err != nil {
		t.Fatal(err)
	}
	command := pluginmcp.CommandID(id)
	if method.Descriptor().Kind == portable.Query {
		command = ""
	}
	value, err := bound.Invoke(ctx, command, input)
	if err != nil {
		return zero, err
	}
	err = json.Unmarshal(value.Bytes(), &zero)
	return zero, err
}

func TestReviewSurvivesRestartAndKeepsOriginalHumanPermit(t *testing.T) {
	db := filepath.Join(t.TempDir(), "reviews.db")
	session, _, closeBackend := connectLearning(t, db)
	propose, _ := skilllearning.ProposeMethod()
	approve, _ := skilllearning.ApproveMethod()
	reject, _ := skilllearning.RejectMethod()
	confirm, _ := skilllearning.ConfirmMethod()
	queue, _ := skilllearning.QueueMethod()
	draft := skilllearning.Draft{Library: "captured-project", Scope: "project", Skill: skilllearning.Skill{Name: "check", Description: "Inspect the project sources", Instructions: "Read the repository."}, Origin: "requested", SourceSession: "ses_parent"}
	first, err := reviewCall(t, session, propose, "initial", extension.Model, draft)
	if err != nil {
		t.Fatal(err)
	}
	draft.Skill.Instructions = "Run the scope checks."
	current, err := reviewCall(t, session, propose, "replacement", extension.Model, draft)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reviewCall(t, session, approve, "stale", extension.Human, first); err == nil {
		t.Fatal("stale proposal was approved")
	}
	if _, err := reviewCall(t, session, approve, "unauthorized", extension.Model, current); err == nil {
		t.Fatal("model approved its own proposal")
	}
	permit, err := reviewCall(t, session, approve, "decision", extension.Human, current)
	if err != nil || permit.Decision != "decision" {
		t.Fatalf("permit = %+v %v", permit, err)
	}
	closeBackend()
	session, _, _ = connectLearning(t, db)
	if _, err := reviewCall(t, session, reject, "withdraw", extension.Human, current); err == nil {
		t.Fatal("withdrawal lost an authorized but unsettled publication")
	}
	again, err := reviewCall(t, session, approve, "resume", extension.Human, current)
	if err != nil || again != permit {
		t.Fatalf("restart rebound permit: %+v %v", again, err)
	}
	pending, err := reviewCall(t, session, queue, "read", extension.Human, skilllearning.Libraries{Project: draft.Library})
	if err != nil || len(pending.Proposals) != 1 || pending.Proposals[0] != current {
		t.Fatalf("recovered queue: %+v %v", pending, err)
	}
	publication := skilllearning.Publication{Proposal: current, Permit: permit, Published: true}
	if _, err := reviewCall(t, session, confirm, "receipt", extension.Human, publication); err != nil {
		t.Fatal(err)
	}
	if _, err := reviewCall(t, session, confirm, "receipt", extension.Human, publication); err != nil {
		t.Fatal(err)
	}
	pending, err = reviewCall(t, session, queue, "reread", extension.Human, skilllearning.Libraries{Project: draft.Library})
	if err != nil || len(pending.Proposals) != 0 {
		t.Fatalf("settled publication still pending: %+v %v", pending, err)
	}
}
