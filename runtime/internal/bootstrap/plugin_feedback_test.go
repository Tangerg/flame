package bootstrap

import (
	json "encoding/json/v2"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
)

func TestFeedbackReceiptAndOriginalAnnotationSurviveWithdrawalAndRestart(t *testing.T) {
	cfg := poMCPConfig(t)
	r := poMCPOpen(t, cfg)
	installed := poMCPActivate(r, filepath.Join(firstPartyPackages, "feedback"))
	waitCallContributorState(t, r, installed.ID, protocol.MCPServerConnected)
	session := r.must(delivery.SessionsCreate, protocol.CreateSessionRequest{}, "new-session").(*protocol.Session)
	in := protocol.FeedbackRequest{SessionID: session.ID, Rating: protocol.FeedbackPositive, Text: " exact text "}
	contribution := installed.Selected.Contributions[0]
	in.Source = &extension.ContributionSource{InstallationID: installed.ID, Release: installed.Selected.Digest, Contribution: contribution.ID}
	stale := in
	other := *in.Source
	other.Release = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	stale.Source = &other
	if res := r.endpoint.Invoke(t.Context(), "feedback.create", stale, delivery.Options{RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.Version}, IdempotencyKey: "stale"}); res.Failure == nil {
		t.Fatal("action silently rebound to a different release")
	}
	r.must("feedback.create", in, "vote")
	r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: installed.ID, Enabled: false}, "disable")
	r = reopenPluginRuntime(t, r, &cfg)
	r.must("feedback.create", in, "vote")
	if result := r.endpoint.Invoke(t.Context(), "feedback.create", in, delivery.Options{RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.Version}, IdempotencyKey: "second-vote"}); result.Failure == nil {
		t.Fatal("withdrawn provider admitted a new annotation")
	}
	// A fresh read cut still contains the evidence, even though its original author cannot run.
	exported, err := r.api.ExportTrajectory(t.Context(), protocol.ExportTrajectoryRequest{SessionID: session.ID})
	if err != nil {
		t.Fatal(err)
	}
	notes := exported.Trajectory.Annotations
	if len(notes) != 1 || notes[0].Source.InstallationID != installed.ID || notes[0].Source.Release != installed.Selected.Digest || notes[0].Source.ContractReference != pluginsdk.FeedbackReference {
		t.Fatalf("lost or duplicated source evidence: %+v", notes)
	}
	var signal pluginsdk.FeedbackSignal
	if err := json.Unmarshal(notes[0].Content, &signal); err != nil || signal.Text != in.Text || signal.Rating != in.Rating {
		t.Fatalf("changed observation %+v %v", signal, err)
	}
}
