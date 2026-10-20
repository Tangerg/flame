package run

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Tangerg/flame/apps/cli/internal/application/mutation"
	"github.com/Tangerg/flame/apps/cli/internal/application/workbench"
	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring/prompt"
	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring/replay"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/runtime/protocol"
)

type openingProbe struct {
	queries []protocol.GetRunAcceptanceRequest
	starts  []prompt.StartRun
	resumes []conversation.ResumeRun
	lookup  func() (protocol.RunAcceptance, error)
	start   func() (conversation.SegmentStream, error)
}

func (p *openingProbe) GetRunAcceptance(_ context.Context, query protocol.GetRunAcceptanceRequest) (protocol.RunAcceptance, error) {
	p.queries = append(p.queries, query)
	return p.lookup()
}

func (p *openingProbe) StartRun(_ context.Context, command prompt.StartRun) (conversation.SegmentStream, error) {
	p.starts = append(p.starts, command.Clone())
	return p.start()
}

func (p *openingProbe) ResumeRun(_ context.Context, command conversation.ResumeRun) (conversation.SegmentStream, error) {
	p.resumes = append(p.resumes, command.Clone())
	return conversation.SegmentStream{}, nil
}

func TestOpeningLookupRemainsReadOnlyAfterDeliveryExpiry(t *testing.T) {
	for _, acceptance := range []protocol.RunAcceptanceType{protocol.RunAcceptanceAccepted, protocol.RunAcceptanceRetired, protocol.RunAcceptanceNotRecorded} {
		t.Run(string(acceptance), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				policy := testReplayPolicy(t)
				guard, err := policy.NewGuard()
				if err != nil {
					t.Fatal(err)
				}
				time.Sleep(2 * time.Hour)
				command := prompt.StartRun{CommandID: mutation.NewCommandID(), SessionID: "ses_original", Message: prompt.Message{Text: "frozen"}}
				receipt := protocol.RunAcceptance{Type: acceptance}
				if acceptance != protocol.RunAcceptanceNotRecorded {
					receipt.RunID, receipt.SegmentID, receipt.SessionID, receipt.UserItemID = "run_original", "seg_original", command.SessionID, "item_original"
				}
				if acceptance == protocol.RunAcceptanceAccepted {
					receipt.Run = &protocol.RunRef{RunSummary: protocol.RunSummary{ID: receipt.RunID, SessionID: receipt.SessionID, Status: protocol.RunStatusRunning}, ActiveSegmentID: "seg_later"}
				}
				probe := &openingProbe{lookup: func() (protocol.RunAcceptance, error) { return receipt, nil }}
				stream, err := OpenStart(t.Context(), probe, command, guard, policy)
				wantQuery := protocol.GetRunAcceptanceRequest{Namespace: guard.Namespace(), Key: string(command.CommandID), Operation: protocol.RunCommandStart}
				if !slices.Equal(probe.queries, []protocol.GetRunAcceptanceRequest{wantQuery}) || len(probe.starts) != 0 {
					t.Fatalf("lookup mutated delivery: queries=%+v starts=%+v", probe.queries, probe.starts)
				}
				switch acceptance {
				case protocol.RunAcceptanceAccepted:
					if err != nil || stream.SegmentID != "seg_original" || stream.RunID != "run_original" {
						t.Fatalf("lookup rewrote the original opening: %+v, %v", stream, err)
					}
					for event, err := range stream.Events {
						if event.Event != nil || !RecoveryRequired(err) {
							t.Fatalf("lookup fabricated a lifecycle frame: %+v, %v", event, err)
						}
					}
				case protocol.RunAcceptanceRetired:
					if _, ok := errors.AsType[*conversation.RetiredOpeningError](err); !ok || stream.Events != nil {
						t.Fatalf("retired material was exposed for observation: %+v, %v", stream, err)
					}
					proof, accepted := conversation.AcceptedMutationReceipt(err)
					if !accepted || proof.RunID != receipt.RunID || proof.SegmentID != receipt.SegmentID {
						t.Fatalf("retired acceptance was lost: %+v, %v", proof, err)
					}
				case protocol.RunAcceptanceNotRecorded:
					if !errors.Is(err, mutation.ErrReplayGuaranteeUnavailable) || !mutation.OutcomeUnknown(err) {
						t.Fatalf("notRecorded settled an expired command: %v", err)
					}
				}
			})
		})
	}
}

func TestOpeningRechecksAdmissionAfterLookup(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		policy := testReplayPolicy(t)
		guard, err := policy.NewGuard()
		if err != nil {
			t.Fatal(err)
		}
		probe := &openingProbe{lookup: func() (protocol.RunAcceptance, error) {
			time.Sleep(2 * time.Hour)
			return protocol.RunAcceptance{Type: protocol.RunAcceptanceNotRecorded}, nil
		}}
		_, err = OpenStart(t.Context(), probe, prompt.StartRun{CommandID: mutation.NewCommandID(), SessionID: "ses_original"}, guard, policy)
		if !errors.Is(err, mutation.ErrReplayGuaranteeUnavailable) || len(probe.starts) != 0 {
			t.Fatalf("expired command crossed delivery admission: %v, %+v", err, probe.starts)
		}
	})
}

func TestOpeningQueryFailureCannotBecomeARefusal(t *testing.T) {
	policy := testReplayPolicy(t)
	guard, err := policy.NewGuard()
	if err != nil {
		t.Fatal(err)
	}
	failure := errors.New("acceptance response is invalid")
	probe := &openingProbe{lookup: func() (protocol.RunAcceptance, error) { return protocol.RunAcceptance{}, failure }}
	_, err = OpenStart(t.Context(), probe, prompt.StartRun{CommandID: mutation.NewCommandID()}, guard, policy)
	if !mutation.OutcomeUnknown(err) || !errors.Is(err, failure) || len(probe.starts) != 0 {
		t.Fatalf("query failure changed command outcome: %v, %+v", err, probe.starts)
	}
}

func TestOpeningLostAcknowledgementQueriesTheOriginalPreparedCommand(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		policy := testReplayPolicy(t)
		guard, err := policy.NewGuard()
		if err != nil {
			t.Fatal(err)
		}
		command := prompt.StartRun{CommandID: mutation.NewCommandID(), SessionID: "ses_original", Message: prompt.Message{Text: "frozen"}, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "frozen"}}}
		probe := &openingProbe{}
		probe.lookup = func() (protocol.RunAcceptance, error) {
			if len(probe.starts) == 0 {
				return protocol.RunAcceptance{Type: protocol.RunAcceptanceNotRecorded}, nil
			}
			return protocol.RunAcceptance{Type: protocol.RunAcceptanceAccepted, RunID: "run_original", SegmentID: "seg_original", SessionID: command.SessionID, UserItemID: "item_original"}, nil
		}
		probe.start = func() (conversation.SegmentStream, error) {
			return conversation.SegmentStream{}, conversation.ErrDisconnected
		}
		opened, err := mutation.ConfirmAdmitted(t.Context(), mutation.AcknowledgementBackoff(), nil, func(ctx context.Context) (conversation.SegmentStream, error) {
			return OpenStart(ctx, probe, command, guard, policy)
		})
		if err != nil || opened.RunID != "run_original" || len(probe.starts) != 1 || !probe.starts[0].Equal(command) || len(probe.queries) != 2 {
			t.Fatalf("lost acknowledgement redelivered or changed input: opened=%+v err=%v starts=%+v queries=%+v", opened, err, probe.starts, probe.queries)
		}
		if probe.queries[0] != probe.queries[1] || probe.queries[0].Namespace != guard.Namespace() || probe.queries[0].Key != string(command.CommandID) {
			t.Fatalf("lookup changed original command identity: %+v", probe.queries)
		}
	})
}

func TestResumeLookupCannotSettleAnotherRunOrDifferentInput(t *testing.T) {
	policy := testReplayPolicy(t)
	guard, err := policy.NewGuard()
	if err != nil {
		t.Fatal(err)
	}
	for _, receipt := range []protocol.RunAcceptance{
		{Type: protocol.RunAcceptanceRetired, RunID: "run_other", SegmentID: "seg_original"},
		{Type: protocol.RunAcceptanceAccepted, RunID: "run_original", SegmentID: "seg_original", UserItemID: "item_unexpected"},
	} {
		probe := &openingProbe{lookup: func() (protocol.RunAcceptance, error) { return receipt, nil }}
		_, err := OpenResume(t.Context(), probe, conversation.ResumeRun{CommandID: mutation.NewCommandID(), RunID: "run_original"}, guard, policy)
		if !mutation.OutcomeUnknown(err) || len(probe.resumes) != 0 || probe.queries[0].Operation != protocol.RunCommandResume {
			t.Fatalf("mismatched receipt settled resume: %v, %+v", err, probe)
		}
	}
}

func TestPendingOpeningLooksUpAcceptanceBeforeRequiringPreparedInput(t *testing.T) {
	policy := testReplayPolicy(t)
	guard, err := policy.NewGuard()
	if err != nil {
		t.Fatal(err)
	}
	message := prompt.Message{Text: "frozen input", Attachments: []prompt.Attachment{{ID: "image", Name: "image.png", Path: "/absent/image.png", MimeType: "image/png", Kind: protocol.ContentBlockImage, Size: 1}}}
	for _, accepted := range []bool{true, false} {
		t.Run(string(protocol.RunCommandStart)+" accepted="+fmt.Sprint(accepted), func(t *testing.T) {
			pending := workbench.PendingRun{State: workbench.PendingRunDispatching, Command: prompt.StartRun{CommandID: mutation.NewCommandID(), SessionID: "ses_original", Message: message}, Replay: guard}
			probe := &openingProbe{lookup: func() (protocol.RunAcceptance, error) {
				if !accepted {
					return protocol.RunAcceptance{Type: protocol.RunAcceptanceNotRecorded}, nil
				}
				return protocol.RunAcceptance{Type: protocol.RunAcceptanceAccepted, RunID: "run_original", SegmentID: "seg_original", SessionID: "ses_original", UserItemID: "item_original"}, nil
			}}
			_, err := OpenPendingStart(t.Context(), probe, pending, policy)
			if len(probe.queries) != 1 || len(probe.starts) != 0 || (accepted && err != nil) || (!accepted && !errors.Is(err, replay.ErrCommandInputUnavailable)) {
				t.Fatalf("input availability changed acceptance lookup: queries=%+v starts=%+v err=%v", probe.queries, probe.starts, err)
			}
		})
		t.Run(string(protocol.RunCommandResume)+" accepted="+fmt.Sprint(accepted), func(t *testing.T) {
			pending := workbench.PendingResume{Command: conversation.ResumeRun{CommandID: mutation.NewCommandID(), RunID: "run_original", Message: &message}, Replay: guard}
			probe := &openingProbe{lookup: func() (protocol.RunAcceptance, error) {
				if !accepted {
					return protocol.RunAcceptance{Type: protocol.RunAcceptanceNotRecorded}, nil
				}
				return protocol.RunAcceptance{Type: protocol.RunAcceptanceAccepted, RunID: "run_original", SegmentID: "seg_original", SessionID: "ses_original", UserItemID: "item_original"}, nil
			}}
			_, err := OpenPendingResume(t.Context(), probe, pending, policy)
			if len(probe.queries) != 1 || len(probe.resumes) != 0 || (accepted && err != nil) || (!accepted && !errors.Is(err, replay.ErrCommandInputUnavailable)) {
				t.Fatalf("input availability changed acceptance lookup: queries=%+v resumes=%+v err=%v", probe.queries, probe.resumes, err)
			}
		})
	}
}
