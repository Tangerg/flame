package bootstrap

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/skilllearning"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/testsupport"

	framecall "github.com/Tangerg/frame/call"
	framevalue "github.com/Tangerg/frame/value"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
)

func TestFilewatchResumesTheSamePartialDataMethodWithPinnedInputs(t *testing.T) {
	cfg := poMCPConfig(t)
	cfg.approvalMode = approval.ModeYolo
	model := &filewatchDataModel{}
	client, err := chatclient.New(delegateRestartModel{chat.ModelFunc(model.call)})
	if err != nil {
		t.Fatal(err)
	}
	cfg.modelProviders = testChatResolver(client)
	fault := &repeatedFilewatchEvents{t: t}
	r := poMCPOpenWithCallbackRPC(t, cfg, fault.wrap)
	watch := poMCPActivate(r, filepath.Join(firstPartyPackages, "filewatch"))
	skills := r.must(delivery.PluginsInstall, protocol.InstallPluginRequest{Source: filepath.Join(firstPartyPackages, "skills")}, "install-skills").(*protocol.PluginInstallation)
	r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: skills.ID, Enabled: true}, "enable-skills")
	r.must(delivery.PluginsSelectProvider, protocol.SelectPluginProviderRequest{Role: skilllearning.Reference, InstallationID: skills.ID, Contribution: "skill-learning"}, "select-skills")
	trajectory := r.must(delivery.PluginsInstall, protocol.InstallPluginRequest{Source: filepath.Join(firstPartyPackages, "trajectory")}, "install-trajectory").(*protocol.PluginInstallation)
	r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: trajectory.ID, Enabled: true}, "enable-trajectory")
	artifact := r.must(delivery.PluginsInstall, protocol.InstallPluginRequest{Source: filepath.Join(firstPartyPackages, "deliverables")}, "install-data-chart").(*protocol.PluginInstallation)
	artifact = r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: artifact.ID, Enabled: true}, "enable-data-chart").(*protocol.PluginInstallation)
	waitCallContributorState(t, r, watch.ID, protocol.MCPServerConnected)
	server := &protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginInstallation, InstallationID: artifact.ID}, Name: "deliverables"}
	for _, tool := range r.poMCPTools(server, func(tools []protocol.MCPTool) bool {
		return slices.ContainsFunc(tools, func(tool protocol.MCPTool) bool { return tool.Name == "create" })
	}) {
		if tool.Name == "create" {
			model.create = tool.ModelName
		}
	}
	call := func(installation *protocol.PluginInstallation, contribution, method string, input any, key string, output any) {
		t.Helper()
		index := slices.IndexFunc(installation.Selected.Contributions, func(value protocol.PluginContribution) bool { return value.ID == contribution })
		if index < 0 {
			t.Fatalf("missing %s contribution: %+v", contribution, installation.Selected.Diagnostics)
		}
		contract, err := plugincontract.ParseCapability([]byte(installation.Selected.Contributions[index].Contract))
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := framevalue.EncodeJSON(input)
		if err != nil {
			t.Fatal(err)
		}
		result := r.must(delivery.PluginsCall, protocol.CallPluginRequest{InstallationID: installation.ID, Digest: installation.Selected.Digest, Contribution: contribution, Contract: contract.Identity().String(), Method: method, Input: encoded.String(), Target: framecall.Target{Scope: extension.ScopeInstallation, Reference: installation.ID}}, key).(*protocol.CallPluginResponse)
		if result.Failure != nil {
			t.Fatalf("%s: %+v", method, result.Failure)
		}
		if output != nil {
			if err := json.Unmarshal([]byte(result.Output), output); err != nil {
				t.Fatalf("%s output: %s: %v", method, result.Output, err)
			}
		}
	}
	watchCall := func(method string, input any, key string, output any) {
		t.Helper()
		call(watch, "filewatch-operations", method, input, key, output)
	}
	artifactCall := func(method string, input, output any) {
		t.Helper()
		call(artifact, "deliverables", "flame.deliverables/"+method+".v1", input, "", output)
	}
	wait := func(label string, check func() bool) {
		t.Helper()
		deadline := time.NewTimer(10 * time.Second)
		defer deadline.Stop()
		probe := time.NewTicker(25 * time.Millisecond)
		defer probe.Stop()
		for !check() {
			select {
			case <-deadline.C:
				t.Fatalf("timed out waiting for %s", label)
			case <-probe.C:
			}
		}
	}
	watchPhase := func(phase testsupport.FilewatchStatusPhase, revision string) bool {
		var result testsupport.FilewatchStatus
		watchCall(testsupport.FilewatchStatusV1Method, struct{}{}, "", &result)
		if result.Phase == testsupport.FilewatchStatusPhaseFailed {
			t.Fatalf("file watch failed: %+v", result)
		}
		return result.Phase == phase && result.Revision.String() == revision
	}
	events := func() testsupport.FilewatchEventsV1Output {
		var result testsupport.FilewatchEventsV1Output
		watchCall(testsupport.FilewatchEventsV1Method, struct{}{}, "", &result)
		return result
	}
	read := func(id string) testsupport.FilewatchEvent {
		var result testsupport.FilewatchEvent
		watchCall(testsupport.FilewatchEventV1Method, testsupport.FilewatchEventInput{Id: id}, "", &result)
		return result
	}
	root := r.must(delivery.WorkspacesResolve, protocol.ResolveWorkspaceRequest{Ref: &protocol.WorkspaceRef{Path: cfg.defaultWorkspacePath}}, "").(*protocol.WorkspaceInfo)
	discovered := r.must(delivery.SkillsDiscoveredList, protocol.WorkspaceQuery{Workspace: root.Ref}, "").(*protocol.SkillDiscovery)
	if !slices.ContainsFunc(discovered.Skills, func(value protocol.Skill) bool { return value.Name == "inspect-history" }) {
		t.Fatalf("default trajectory Skill unavailable: %+v", discovered)
	}
	session := r.must(delivery.SessionsCreate, protocol.CreateSessionRequest{Title: "P1 analysis", Workspace: &root.Ref}, "data-session").(*protocol.Session)
	east := filepath.Join(root.Ref.Path, "east.csv")
	west := filepath.Join(root.Ref.Path, "west.csv")
	write := func(path, text string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(east, "P1,0\nP2,80")
	write(west, "P1,6\nP2,90")
	const instructions = "M1. Filter P1 and generate a new editable chart for these inputs. Include exact source hashes and the read/required denominator. Ask before proceeding with partial data."
	watchCall(testsupport.FilewatchConfigureV1Method, testsupport.FilewatchConfigureInput{
		ExpectedRevision: generatedNumber(t, "1"), Trigger: &testsupport.FilewatchConfigureV1InputFilewatchDraft{
			Title: "M1", Workspace: root.Ref.Path, Paths: []string{"east.csv", "west.csv"}, Instructions: instructions, TargetSessionId: &session.ID, CapturePartitions: new(true),
		},
	}, "follow-data", nil)
	wait("active method", func() bool { return watchPhase(testsupport.FilewatchStatusPhaseWatching, "2") })
	write(east, "P1,2\nP2,80")
	wait("admission of D1", func() bool {
		values := events()
		if len(values) == 1 && values[0].Accepted {
			return true
		}
		var status testsupport.FilewatchStatus
		watchCall(testsupport.FilewatchStatusV1Method, struct{}{}, "", &status)
		if status.DispatchError != nil {
			t.Fatalf("initial D1 dispatch failed: %s", *status.DispatchError)
		}
		return false
	})
	d1 := events()[0]
	waitForProtocolRunTerminal(t, t.Context(), r.api, d1.RunId)
	if run := r.must(delivery.RunsGet, protocol.GetRunRequest{RunID: d1.RunId}, "").(*protocol.RunRef); run.Outcome == nil || run.Outcome.Type != protocol.OutcomeCompleted {
		t.Fatalf("D1 did not create its chart: %+v", run.Outcome)
	}
	if err := os.Remove(west); err != nil {
		t.Fatal(err)
	}
	wait("admission and repeated reads for D2", func() bool {
		values := events()
		return len(values) >= 2 && values[0].Accepted && fault.reads.Load() >= 6
	})
	d2 := events()[0]
	if d2.RunId == d1.RunId || d1.SessionId != session.ID || d2.SessionId != session.ID || len(events()) != 2 {
		t.Fatalf("follow changed Session or repeated an opening: %+v", events())
	}
	if d2.Kind != "resync" {
		t.Fatalf("observation gap did not trigger a full resync: %+v", d2)
	}
	wait("original D2 Run awaiting input", func() bool {
		run := r.must(delivery.RunsGet, protocol.GetRunRequest{RunID: d2.RunId}, "").(*protocol.RunRef)
		if run.Outcome != nil {
			t.Fatalf("partial Run ended before the question: %+v", run.Outcome)
		}
		return run.Status == protocol.RunStatusWaiting
	})
	first, second := read(d1.Id), read(d2.Id)
	if first.Capture.Partitions == nil || second.Capture.Partitions == nil {
		t.Fatal("observations lost their partition evidence")
	}
	selected := *second.Capture.Partitions
	previous := *first.Capture.Partitions
	if len(selected) != 2 || len(previous) != 2 || selected[0].State != "read" || selected[0].Sha256 == nil || previous[0].Sha256 == nil ||
		*selected[0].Sha256 != *previous[0].Sha256 || selected[1].State != "missing" || previous[1].State != "read" ||
		second.Capture.Revision.String() != "2" || second.Capture.Trigger.Instructions != instructions {
		t.Fatalf("D2 lost its pinned inputs or used the D1 denominator: D1=%+v D2=%+v", first.Capture, second.Capture)
	}
	watchCall(testsupport.FilewatchConfigureV1Method, testsupport.FilewatchConfigureInput{ExpectedRevision: generatedNumber(t, "2")}, "stop-future-data", nil)
	wait("disabled rule", func() bool { return watchPhase(testsupport.FilewatchStatusPhaseDisabled, "3") })
	// The reopened Run must still use the capture from before the subscriber stopped.
	write(east, "P1,99\nP2,80")
	r = reopenPluginRuntime(t, r, &cfg)
	waitCallContributorState(t, r, watch.ID, protocol.MCPServerConnected)
	waitCallContributorState(t, r, artifact.ID, protocol.MCPServerConnected)
	meta := protocol.RequestMeta{ProtocolVersion: protocol.Version, ClientCapabilities: &protocol.ClientCapabilities{
		InterruptTypes: []protocol.InterruptType{protocol.InterruptQuestion, protocol.InterruptApproval},
	}}
	ctx := delivery.WithRequestMeta(t.Context(), meta)
	pending, err := r.api.ListInterrupts(ctx, protocol.ListInterruptsRequest{RootRunID: d2.RunId})
	if err != nil || len(pending.Data) != 1 || len(pending.Data[0].Interrupts) != 1 {
		t.Fatalf("original partial Run lost the question: %+v, %v", pending, err)
	}
	question := pending.Data[0].Interrupts[0]
	resumed := r.endpoint.Invoke(ctx, delivery.RunsResume, protocol.ResumeRunRequest{RunID: d2.RunId, Responses: []protocol.InterruptResponse{{
		ItemID: question.ItemID, Response: protocol.InterruptResponseValue{Type: protocol.InterruptResponseAnswer, Answers: [][]string{{"Use available partitions"}}},
	}}}, protocol.CallMetadata{IdempotencyKey: "resume-d2", RequestMeta: meta})
	if resumed.Failure != nil {
		t.Fatal(resumed.Failure)
	}
	for _, err := range resumed.Events {
		if err != nil {
			t.Fatal(err)
		}
	}
	if run := r.must(delivery.RunsGet, protocol.GetRunRequest{RunID: d2.RunId}, "").(*protocol.RunRef); run.Outcome == nil || run.Outcome.Type != protocol.OutcomeCompleted {
		t.Fatalf("retained D2 Run did not complete: %+v", run)
	}
	if values := events(); len(values) != 2 || values[0].RunId != d2.RunId {
		t.Fatalf("stopped rule or repeated events dispatched again: %+v", values)
	}
	var list struct {
		Entries []struct {
			ID    string `json:"id"`
			Title string `json:"title"`
			Head  string `json:"head"`
		} `json:"entries"`
	}
	artifactCall("list", map[string]any{"limit": 10}, &list)
	if len(list.Entries) != 2 {
		t.Fatalf("expected exactly the two original chart outputs, got %+v", list.Entries)
	}
	for _, entry := range list.Entries {
		var chart struct {
			LayoutJSON string `json:"layoutJSON"`
		}
		artifactCall("slides", map[string]string{"id": entry.ID, "revision": entry.Head}, &chart)
		var layout struct {
			Slides []struct {
				Notes []struct {
					Text string `json:"text"`
				} `json:"notes"`
				Elements []struct {
					Chart struct {
						Data struct {
							Series []struct {
								Values []int `json:"values"`
							} `json:"series"`
						} `json:"data"`
					} `json:"chart"`
				} `json:"elements"`
			} `json:"slides"`
		}
		if err := json.Unmarshal([]byte(chart.LayoutJSON), &layout); err != nil || len(layout.Slides) != 1 || len(layout.Slides[0].Elements) != 1 || len(layout.Slides[0].Elements[0].Chart.Data.Series) != 1 {
			t.Fatalf("native chart missing for %s: %s %v", entry.Title, chart.LayoutJSON, err)
		}
		page := layout.Slides[0]
		values := page.Elements[0].Chart.Data.Series[0].Values
		var notes strings.Builder
		for _, part := range page.Notes {
			notes.WriteString(part.Text)
		}
		switch entry.Title {
		case "D1 P1":
			if !slices.Equal(values, []int{2, 6}) || !strings.Contains(notes.String(), "2/2") {
				t.Fatalf("D1 values and evidence %v %s", values, notes.String())
			}
		case "D2 P1 (partial)":
			if !slices.Equal(values, []int{2}) || !strings.Contains(notes.String(), "1/2") ||
				!strings.Contains(notes.String(), "missing west.csv") || !strings.Contains(notes.String(), *selected[0].Sha256) {
				t.Fatalf("D2 lost its missing partition or reused the wrong data: %v %s", values, notes.String())
			}
		default:
			t.Fatalf("unexpected chart %q", entry.Title)
		}
	}
}

type filewatchDataModel struct{ create string }

func (m *filewatchDataModel) call(_ context.Context, request *chat.Request) (*chat.Response, error) {
	if len(request.Tools) == 0 {
		return completedTextResponse("P1 analysis"), nil
	}
	var input string
	results := map[string]*chat.ToolResult{}
	for _, message := range request.Messages {
		if message.Role == chat.RoleUser && strings.Contains(message.Text(), "<flame-file-observation>") {
			input = message.Text()
		}
		for _, part := range message.Parts {
			if part.ToolResult != nil {
				results[part.ToolResult.ID] = part.ToolResult
			}
		}
	}
	var observation struct {
		Revision int `json:"methodRevision"`
		Coverage struct {
			Expected int `json:"expected"`
			Read     int `json:"read"`
			Inputs   []struct {
				Path   string `json:"path"`
				State  string `json:"state"`
				Text   string `json:"text"`
				SHA256 string `json:"sha256"`
			} `json:"inputs"`
		} `json:"partitionCoverage"`
	}
	_, payload, ok := strings.Cut(input, "<flame-file-observation>\n")
	payload, _, _ = strings.Cut(payload, "\n</flame-file-observation>")
	if err := json.Unmarshal([]byte(payload), &observation); !ok || err != nil || observation.Revision != 2 || observation.Coverage.Expected != 2 {
		return nil, fmt.Errorf("unexpected method or evidence %q: %v", input, err)
	}
	facts := observation.Coverage
	period, title := "d1", "D1 P1"
	if facts.Read < facts.Expected {
		period, title = "d2", "D2 P1 (partial)"
		if results["partial-confirmation"] == nil {
			return filewatchToolCall(chat.ToolCall{ID: "partial-confirmation", Name: "ask_user", Arguments: `{"questions":[{"question":"Use available partitions?"}]}`})
		}
	}
	if result := results["create-"+period]; result != nil {
		text, _ := result.Output.Text()
		if result.IsError || strings.Contains(text, `"failure"`) {
			return nil, fmt.Errorf("native chart was refused: %s", text)
		}
		return completedTextResponse(title + " retained"), nil
	}
	notes := fmt.Sprintf("M1 v%d; P1; %d/%d", observation.Revision, facts.Read, facts.Expected)
	var labels []string
	var values []int
	sources := []map[string]string{}
	for _, partition := range facts.Inputs {
		if partition.State != "read" {
			notes += fmt.Sprintf("; %s %s", partition.State, partition.Path)
			continue
		}
		for line := range strings.SplitSeq(partition.Text, "\n") {
			key, number, _ := strings.Cut(line, ",")
			if key != "P1" {
				continue
			}
			value, err := strconv.Atoi(number)
			if err != nil {
				return nil, err
			}
			labels, values = append(labels, partition.Path), append(values, value)
		}
		notes += "; " + partition.Path + " " + partition.SHA256
		sources = append(sources, map[string]string{"title": partition.Path, "uri": partition.Path, "note": partition.SHA256})
	}
	chart := map[string]any{"type": "chart", "chart": "bar", "title": title, "x": 1, "y": 2, "width": 8, "height": 4,
		"series": []any{map[string]any{"name": "P1", "labels": labels, "values": values}}}
	content, err := json.Marshal(map[string]any{"title": title, "slides": []any{map[string]any{"title": title, "notes": notes, "elements": []any{chart}}}})
	if err != nil {
		return nil, err
	}
	args, err := json.Marshal(map[string]any{"title": title, "format": "pptx", "contentJSON": string(content), "provenance": map[string]any{"sources": sources, "assumptions": []string{}, "units": "count", "reproduction": notes}})
	if err != nil {
		return nil, err
	}
	return filewatchToolCall(deferredToolCall(request, "discover-"+period, chat.ToolCall{ID: "create-" + period, Name: m.create, Arguments: string(args)}))
}

func filewatchToolCall(call chat.ToolCall) (*chat.Response, error) {
	message := chat.NewAssistantMessage(chat.NewToolCallPart(call))
	return chat.NewResponse(&chat.Output{Message: &message, FinishReason: chat.FinishReasonToolCalls}, nil)
}

type repeatedFilewatchEvents struct {
	t     *testing.T
	reads atomic.Int32
}

func (f *repeatedFilewatchEvents) wrap(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			f.t.Error(err)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		var request struct {
			Method string `json:"method"`
		}
		if err := json.Unmarshal(body, &request); err != nil {
			f.t.Error(err)
			return
		}
		if request.Method == "runtime.subscribe" {
			w = &repeatedFilewatchWriter{ResponseWriter: w}
		}
		if request.Method == "workspace.files.read" {
			defer f.reads.Add(1)
		}
		handler.ServeHTTP(w, r)
	})
}

type repeatedFilewatchWriter struct {
	http.ResponseWriter
	previous []byte
}

func (w *repeatedFilewatchWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *repeatedFilewatchWriter) Write(body []byte) (int, error) {
	data := bytes.Index(body, []byte("data:"))
	if data < 0 {
		return w.ResponseWriter.Write(body)
	}
	var frame struct {
		Params protocol.RuntimeEventNotification `json:"params"`
	}
	if json.Unmarshal(bytes.TrimSpace(body[data+5:]), &frame) != nil || frame.Params.Event.Type != protocol.RuntimeFilesChanged {
		return w.ResponseWriter.Write(body)
	}
	if w.previous == nil {
		w.previous = bytes.Clone(body)
		return w.ResponseWriter.Write(body)
	}
	sequence := frame.Params.Event.Sequence
	original := fmt.Appendf(nil, `"sequence":%d`, sequence)
	shifted := bytes.Replace(body, original, fmt.Appendf(nil, `"sequence":%d`, sequence+1), 1)
	for _, repeated := range [][]byte{
		shifted,
		shifted,
		w.previous,
		bytes.Replace(body, original, fmt.Appendf(nil, `"sequence":%d`, sequence+2), 1),
	} {
		if _, err := w.ResponseWriter.Write(repeated); err != nil {
			return 0, err
		}
	}
	return len(body), nil
}
