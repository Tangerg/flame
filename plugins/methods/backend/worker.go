package main

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"reflect"
	"strings"
	"time"

	runtimeclient "github.com/Tangerg/flame/packages/go-sdk/client"
	"github.com/Tangerg/flame/packages/go-sdk/identity"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/plugins/methods/internal/contract"
	"github.com/Tangerg/flame/plugins/methods/internal/wire"
)

type connector struct {
	store store
	wake  chan struct{}
}

func (c *connector) signal() {
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

func (c *connector) setup(ctx context.Context, setup pluginmcp.Setup) (io.Closer, error) {
	for _, entry := range setup.Contributions() {
		if entry.Export != contract.Export || entry.Background == nil {
			continue
		}
		host, err := hostClient(ctx)
		if err != nil {
			return nil, err
		}
		worker, err := pluginmcp.NewBackground(ctx, pluginmcp.BackgroundConfig{
			Run:     func(ctx context.Context) { c.runWorker(ctx, host, entry.ID) },
			Release: host.Close,
		})
		if err != nil {
			return nil, errors.Join(err, host.Close())
		}
		return worker, nil
	}
	return nil, nil
}

func (c *connector) runWorker(ctx context.Context, host *runtimeclient.Client, contribution string) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	dirty := true
	for {
		if changed := c.advanceAll(ctx, host); changed || dirty {
			if err := host.NotifyPluginChange(ctx, protocol.PluginChangeNotice{Contribution: contribution}, protocol.CallOptions{}); err != nil && ctx.Err() == nil {
				slog.WarnContext(ctx, "methods: notify project", "error", err)
			}
		}
		dirty = false
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-c.wake:
			dirty = true
		}
	}
}

func (c *connector) advanceAll(ctx context.Context, host *runtimeclient.Client) bool {
	works, err := c.store.executions(ctx, "SELECT value FROM method_executions WHERE state IN ('queued','changing','reviewing') ORDER BY rowid")
	if err != nil {
		if ctx.Err() == nil {
			slog.WarnContext(ctx, "methods: read pending executions", "error", err)
		}
		return false
	}
	changed := false
	for _, original := range works {
		work := original
		if err := c.advance(ctx, host, &work); err != nil {
			if ctx.Err() != nil {
				return changed
			}
			switch {
			case errors.Is(err, protocol.ErrSessionBusy):
				work.Reason = "Project admission is paused or a Run is using the worktree."
			case errors.Is(err, protocol.ErrIdempotencyStoreMismatch):
				work.State, work.Reason = wire.Blocked, "The original Runtime authority is unavailable."
			default:
				if refusal, known := errors.AsType[rejection](err); known {
					work.State, work.Reason = wire.Blocked, string(refusal)
				} else {
					slog.WarnContext(ctx, "methods: observe execution", "execution", work.ID, "error", err)
					work.Reason = "Waiting to confirm the original Run or evidence."
				}
			}
		}
		if ctx.Err() != nil {
			return changed
		}
		if !reflect.DeepEqual(work, original) {
			if err := c.store.update(ctx, work); err != nil {
				slog.WarnContext(ctx, "methods: retain execution", "execution", work.ID, "error", err)
			} else {
				changed = true
			}
		}
	}
	return changed
}

func (c *connector) advance(ctx context.Context, host *runtimeclient.Client, work *record) error {
	switch work.State {
	case wire.Queued:
		run, err := stageRun(ctx, host, *work, false)
		if err != nil {
			return err
		}
		work.Change, work.State, work.Reason = run, wire.Changing, ""
	case wire.Changing:
		done, err := completedRun(ctx, host, work.Change)
		if err != nil || !done {
			return err
		}
		preview, err := currentWorktree(ctx, host, *work)
		if err != nil {
			return err
		}
		work.Handoff, work.Patch = preview.Digest, preview.Patch
		work.State, work.Reason = wire.Reviewing, ""
	case wire.Reviewing:
		return advanceReview(ctx, host, work)
	}
	return nil
}

func stageRun(ctx context.Context, host *runtimeclient.Client, work record, review bool) (*wire.Run, error) {
	stage := "method-change"
	body := "Make the requested changes in this managed worktree. Apply edits in this workspace and report the results.\n\n" + work.Version.Change + "\n\nOriginal worktree handoff: " + work.Before
	meta := protocol.RequestMeta{ClientCapabilities: &protocol.ClientCapabilities{
		InterruptTypes: []protocol.InterruptType{protocol.InterruptQuestion, protocol.InterruptApproval},
	}}
	if review {
		stage = "method-review"
		assignments, err := json.Marshal(methodAssignments(work))
		if err != nil {
			return nil, err
		}
		body = "Delegate both checks in the same turn. Pass through each summary and instructions exactly. Wait for both original results, then summarize.\n\n" + string(assignments)
		meta.ClientCapabilities.Features = map[string]protocol.FeaturePreference{protocol.FeatureChildRuns: {Enabled: true}}
	}
	entropy, err := methodDigest([2]string{work.ID, stage})
	if err != nil {
		return nil, err
	}
	session, sessionErr := identity.NewSession(entropy)
	run, runErr := identity.NewRun(entropy)
	if err := errors.Join(sessionErr, runErr); err != nil {
		return nil, err
	}
	request := protocol.StartRunRequest{
		NewSession: &protocol.RunSessionOpening{
			SessionID: session.String(), RunID: run.String(), Title: stage + " " + work.Version.Title,
			ProjectID: work.ProjectID, Workspace: &protocol.WorkspaceRef{Path: work.Workspace},
		},
		Provider: work.Version.Provider, Model: work.Version.Model,
		Background: &protocol.BackgroundSource{ProjectID: work.ProjectID, Kind: stage, ID: work.ID, Version: work.Version.ID, Title: work.Version.Title},
		Input:      []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: body}},
	}
	expected := &wire.Run{ID: run.String(), SessionID: session.String()}
	key := stage + ":" + work.ID
	acceptance, err := host.GetRunAcceptance(ctx, protocol.GetRunAcceptanceRequest{
		Namespace: work.Namespace, Key: key, Operation: protocol.RunCommandStart, Request: &request,
	}, protocol.CallOptions{RequestMeta: meta})
	if err != nil {
		return nil, err
	}
	switch acceptance.Type {
	case protocol.RunAcceptanceAccepted:
		if acceptance.RunID != expected.ID || acceptance.SessionID != expected.SessionID {
			return nil, protocol.ErrInvalidResponse
		}
		return expected, nil
	case protocol.RunAcceptanceRetired:
		return nil, refuse("the original method Run was retired")
	case protocol.RunAcceptanceNotRecorded:
		if !review {
			preview, err := currentWorktree(ctx, host, work)
			if err != nil {
				return nil, err
			}
			if work.Before != preview.Digest {
				return nil, refuse("the worktree changed before the method began")
			}
		}
	default:
		return nil, protocol.ErrInvalidResponse
	}
	observation, cancel := context.WithCancel(ctx)
	defer cancel()
	reply, _, err := host.StartRun(observation, request, protocol.RunCommandOptions{
		IdempotencyKey: key, IdempotencyNamespace: work.Namespace, RequestMeta: meta,
	})
	if err != nil {
		return nil, err
	}
	if reply.RunID != expected.ID {
		return nil, protocol.ErrInvalidResponse
	}
	return expected, nil
}

func completedRun(ctx context.Context, host *runtimeclient.Client, run *wire.Run) (bool, error) {
	current, err := host.GetRun(ctx, protocol.GetRunRequest{RunID: run.ID}, protocol.CallOptions{})
	if errors.Is(err, protocol.ErrRunNotFound) {
		return false, refuse("the original method Run is unavailable")
	}
	if err != nil {
		return false, err
	}
	if current.ID != run.ID || current.SessionID != run.SessionID {
		return false, protocol.ErrInvalidResponse
	}
	if current.Status != protocol.RunStatusFinished {
		return false, nil
	}
	if current.Outcome == nil || current.Outcome.Type != protocol.OutcomeCompleted {
		return false, refuse("the original method Run did not complete")
	}
	return true, nil
}

type assignment struct {
	Summary      string `json:"summary"`
	Instructions string `json:"instructions"`
}

func methodAssignments(work record) [2]assignment {
	// The exact change identity is passed to each native child. The report keeps
	// the full handoff patch. Reviewers also read the retained worktree.
	handoff := fmt.Sprintf("\n\nReview handoff %s on base %s. Keep the worktree read-only. Begin your final response with PASS or FAIL, followed by the evidence.", work.Handoff, work.Base)
	return [2]assignment{
		{Summary: "Method review A", Instructions: work.Version.ReviewA + handoff},
		{Summary: "Method review B", Instructions: work.Version.ReviewB + handoff},
	}
}

func advanceReview(ctx context.Context, host *runtimeclient.Client, work *record) error {
	preview, err := currentWorktree(ctx, host, *work)
	if err != nil {
		return err
	}
	if preview.Digest != work.Handoff {
		return refuse("the worktree or handoff target changed during review")
	}
	if work.Review == nil {
		run, err := stageRun(ctx, host, *work, true)
		if err != nil {
			return err
		}
		work.Review, work.Reason = run, ""
		return nil
	}
	checks, err := readChecks(ctx, host, *work)
	if err != nil {
		return err
	}
	work.Checks, work.Reason = checks, ""
	done, err := completedRun(ctx, host, work.Review)
	if err != nil || !done {
		return err
	}
	if len(checks) != 2 || checks[0].Status != "passed" || checks[1].Status != "passed" {
		return refuse("both original reviewers must return passing evidence")
	}
	digest, err := methodDigest(struct {
		Execution, Version, Handoff string
		Checks                      []wire.Review
	}{work.ID, work.Version.ID, work.Handoff, checks})
	if err != nil {
		return err
	}
	work.Digest, work.State = digest, wire.Awaiting
	return nil
}

func readChecks(ctx context.Context, host *runtimeclient.Client, work record) ([]wire.Review, error) {
	options := protocol.CallOptions{RequestMeta: protocol.RequestMeta{ClientCapabilities: &protocol.ClientCapabilities{
		Features: map[string]protocol.FeaturePreference{protocol.FeatureChildRuns: {Enabled: true}},
	}}}
	page, err := host.ListRuns(ctx, protocol.ListRunsRequest{
		SessionID: work.Review.SessionID, IncludeDescendants: true, PageQuery: protocol.PageQuery{Limit: new(100)},
	}, options)
	if err != nil {
		return nil, err
	}
	if len(page.Data) <= 1 {
		return []wire.Review{}, nil
	}
	if len(page.Data) > 3 || page.NextCursor != "" {
		return nil, refuse("the original review launched additional Runs")
	}
	parentItems, err := runItems(ctx, host, work.Review.ID, options)
	if err != nil {
		return nil, err
	}
	planned := methodAssignments(work)
	results := make(map[string]wire.Review, 2)
	for _, run := range page.Data {
		if run.ID == work.Review.ID {
			continue
		}
		if run.ParentRunID != work.Review.ID || run.SessionID != work.Review.SessionID {
			return nil, refuse("a review Run is outside the original parent")
		}
		var slot string
		for _, item := range parentItems {
			if item.ID != run.SpawnedByItemID || item.Tool == nil || item.Tool.Name != "delegate_task" {
				continue
			}
			summary, _ := item.Tool.Arguments["summary"].(string)
			instructions, _ := item.Tool.Arguments["instructions"].(string)
			for index, expected := range planned {
				if summary == expected.Summary && instructions == expected.Instructions {
					slot = []string{"A", "B"}[index]
					break
				}
			}
		}
		if _, duplicate := results[slot]; duplicate || slot == "" {
			return nil, refuse("a review Run did not use an original distinct assignment")
		}
		evidence := wire.Review{Slot: slot, RunID: run.ID, Status: string(run.Status)}
		if run.Status == protocol.RunStatusFinished {
			evidence.Status = "failed"
			if run.Outcome != nil && run.Outcome.Type == protocol.OutcomeCompleted {
				items, err := runItems(ctx, host, run.ID, options)
				if err != nil {
					return nil, err
				}
				for _, item := range items {
					if item.Type != protocol.ItemTypeAgentMessage || item.Phase != protocol.MessagePhaseFinalAnswer ||
						item.Status != protocol.ItemStatusCompleted || item.Redacted {
						continue
					}
					var response strings.Builder
					for _, block := range item.Content {
						if block.Type == protocol.ContentBlockText {
							response.WriteString(block.Text)
						}
					}
					result := []rune(response.String())
					evidence.Result = string(result[:min(len(result), 2048)])
					marker, _, _ := strings.Cut(strings.TrimSpace(evidence.Result), "\n")
					if marker == "PASS" || strings.HasPrefix(marker, "PASS ") || strings.HasPrefix(marker, "PASS:") {
						evidence.Status = "passed"
					}
					break
				}
			}
		}
		results[slot] = evidence
	}
	checks := make([]wire.Review, 0, len(results))
	for _, slot := range []string{"A", "B"} {
		if result, known := results[slot]; known {
			checks = append(checks, result)
		}
	}
	return checks, nil
}

func runItems(ctx context.Context, host *runtimeclient.Client, run string, options protocol.CallOptions) ([]protocol.Item, error) {
	var items []protocol.Item
	var cursor string
	for {
		page, err := host.ListItems(ctx, protocol.ListItemsRequest{
			Scope: protocol.ItemListScope{Type: protocol.ItemScopeRun, RunID: run},
			Order: protocol.ItemOrderDesc, PageQuery: protocol.PageQuery{Limit: new(100), Cursor: cursor},
		}, options)
		if err != nil {
			return nil, err
		}
		items = append(items, page.Data...)
		if page.NextCursor == "" {
			return items, nil
		}
		if page.NextCursor == cursor {
			return nil, protocol.ErrInvalidResponse
		}
		cursor = page.NextCursor
	}
}
