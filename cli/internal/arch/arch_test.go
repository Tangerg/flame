// Package arch holds the tests that keep this program's layering true.
//
// The interface library lives in its own module and guards its own rings. What is
// guarded here is the product: where its data comes from, how it is folded, how it is
// shown, and the rule that keeps the library a library.
package arch

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestGoalProjectionRequiresImmutableRestoration(t *testing.T) {
	root := moduleRoot(t)
	goalPath := filepath.Join(root, "internal", "domain", "agent", "goal.go")
	wantGoal := map[string]string{
		"sessionID": "string", "objective": "string", "status": "GoalStatus", "reason": "*GoalReason",
		"provider": "string", "model": "string", "budget": "GoalBudget", "used": "GoalUsage",
		"createdAt": "time.Time", "updatedAt": "time.Time",
	}
	if fields := cliStructFieldTypes(t, goalPath, "Goal"); !maps.Equal(fields, wantGoal) {
		t.Fatalf("agent.Goal fields = %v, want private immutable state %v", fields, wantGoal)
	}
	if fields := cliStructFieldTypes(t, goalPath, "GoalReason"); !maps.Equal(fields, map[string]string{
		"code": "GoalReasonCode", "detail": "string",
	}) {
		t.Fatalf("agent.GoalReason fields = %v, want private reason state", fields)
	}
	if fields := cliStructFieldTypes(t, goalPath, "GoalUsage"); !maps.Equal(fields, map[string]string{
		"runs": "int", "costUSD": "float64", "steps": "int",
	}) {
		t.Fatalf("agent.GoalUsage fields = %v, want private accounting state", fields)
	}

	adapterPath := filepath.Join(root, "internal", "adapter", "runtimebinding", "goals.go")
	contents, err := os.ReadFile(adapterPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(contents)
	for _, required := range []string{"agent.NewGoalUsage(", "agent.RestoreGoal(snapshot)"} {
		if !strings.Contains(text, required) {
			t.Errorf("embedded Goal adapter lost validated construction %q", required)
		}
	}
	if strings.Contains(text, "projected := agent.Goal{") {
		t.Fatal("embedded Goal adapter restored caller-shaped aggregate construction")
	}
}

func TestWorkspaceRequestModelsOwnOptionalPositiveIntent(t *testing.T) {
	root := moduleRoot(t)
	files := filepath.Join(root, "internal", "domain", "workspace", "files.go")
	policies := filepath.Join(root, "internal", "domain", "workspace", "file_request_policy.go")
	diff := filepath.Join(root, "internal", "domain", "workspace", "diff.go")

	checks := []struct {
		path      string
		structure string
		field     string
		want      string
	}{
		{path: diff, structure: "DiffRequest", field: "RowLimit", want: "DiffRowLimit"},
		{path: files, structure: "HeadRequest", field: "LineLimit", want: "HeadLineLimit"},
		{path: files, structure: "SearchRequest", field: "Limit", want: "SearchResultLimit"},
		{path: files, structure: "ReadRequest", field: "Range", want: "ReadLineRange"},
		{path: files, structure: "ReadRequest", field: "ByteLimit", want: "ReadByteLimit"},
	}
	for _, check := range checks {
		fields := cliStructFieldTypes(t, check.path, check.structure)
		if got := fields[check.field]; got != check.want {
			t.Errorf("workspace.%s.%s type = %q, want %q", check.structure, check.field, got, check.want)
		}
	}
	for _, retired := range []string{"StartLine", "EndLine", "MaxBytes"} {
		if _, exists := cliStructFieldTypes(t, files, "ReadRequest")[retired]; exists {
			t.Errorf("workspace.ReadRequest restored primitive %s", retired)
		}
	}
	if _, exists := cliStructFieldTypes(t, diff, "DiffRequest")["Limit"]; exists {
		t.Error("workspace.DiffRequest restored primitive Limit")
	}
	for _, check := range []struct {
		path      string
		structure string
		value     string
	}{
		{path: policies, structure: "HeadLineLimit", value: "lines"},
		{path: policies, structure: "SearchResultLimit", value: "matches"},
		{path: policies, structure: "ReadByteLimit", value: "bytes"},
		{path: diff, structure: "DiffRowLimit", value: "rows"},
	} {
		fields := cliStructFieldTypes(t, check.path, check.structure)
		if fields["kind"] != "requestLimitKind" || fields[check.value] != "int" {
			t.Errorf("workspace.%s fields = %v, want private kind/%s", check.structure, fields, check.value)
		}
		if _, exists := fields["explicit"]; exists {
			t.Errorf("workspace.%s restored a bool/zero-value mode", check.structure)
		}
	}
	rangeFields := cliStructFieldTypes(t, policies, "ReadLineRange")
	if rangeFields["kind"] != "readLineRangeKind" {
		t.Fatalf("workspace.ReadLineRange fields = %v, want explicit kind", rangeFields)
	}
}

func TestRuntimeBindingCursorTraversalOwnsFiniteCapacityAndOpaqueIdentity(t *testing.T) {
	root := moduleRoot(t)
	paginationPath := filepath.Join(root, "internal", "adapter", "runtimebinding", "pagination.go")
	contents, err := os.ReadFile(paginationPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(contents)
	for _, required := range []string{
		"maximumPaginationCursorBytes",
		"= protocol.MaximumPaginationCursorCharacters",
		"maximumRequests int",
		"seenFingerprints map[[sha256.Size]byte]struct{}",
		"cursorFingerprint(next)",
		"c.pageRequests >= c.maximumRequests",
		"validateRequestCursor(operation, initial)",
	} {
		if !strings.Contains(text, required) {
			t.Errorf("cursor traversal lost bounded protocol rule %q", required)
		}
	}
	if strings.Contains(text, "func newCursorTraversal(operation, initial string) *cursorTraversal") {
		t.Fatal("cursor traversal restored its unbounded constructor")
	}

	for _, caller := range []struct {
		path   string
		policy string
	}{
		{path: "schedules.go", policy: "maximumSchedulePageRequests"},
		{path: "workspaces.go", policy: "maximumWorkspaceFilePageRequests"},
	} {
		path := filepath.Join(root, "internal", "adapter", "runtimebinding", caller.path)
		contents, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(contents), caller.policy) ||
			!strings.Contains(string(contents), "newCursorTraversal(") {
			t.Errorf("%s does not publish and consume %s", caller.path, caller.policy)
		}
	}
}

func TestRunReplayCursorUsesThePublicResourceAndFramingContract(t *testing.T) {
	contents, err := os.ReadFile(filepath.Join(moduleRoot(t), "internal", "adapter", "runtimebinding", "connection.go"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(contents)
	for _, required := range []string{
		"protocol.MaximumRunEventIDCharacters",
		"protocol.IDPrefixEvent",
		"func (r *Connection) subscriptionOptions(afterEventID string) (flameruntime.RunSubscriptionOptions, error)",
	} {
		if !strings.Contains(text, required) {
			t.Errorf("Run replay adapter lost public contract %q", required)
		}
	}
}

func TestSessionCatalogFiltersStayAtTheRuntimeQueryBoundary(t *testing.T) {
	root := moduleRoot(t)
	path := filepath.Join(root, "internal", "adapter", "runtimebinding", "sessions.go")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(contents)
	if !strings.Contains(text, "protocol.ListSessionsRequest") ||
		!strings.Contains(text, "Search:    query.Search") ||
		!strings.Contains(text, "request.Workspace = &protocol.WorkspaceRef") {
		t.Fatal("session adapter does not project its filters into sessions.list")
	}
	for _, retired := range []string{
		"func (r *Connection) listFilteredSessions(",
		"func matchesSession(",
		"Limit: protocolPositiveInt(1)",
		"maximumFilteredSessionPageRequests",
	} {
		if strings.Contains(text, retired) {
			t.Errorf("session adapter restored client-side scan %q", retired)
		}
	}
}

func TestUsageSummaryOwnsAllTimeVsRecentPeriod(t *testing.T) {
	root := moduleRoot(t)
	usagePath := filepath.Join(root, "internal", "domain", "agent", "usage_report.go")
	fields := cliStructFieldTypes(t, usagePath, "UsageSummary")
	if got := fields["Period"]; got != "UsageSummaryPeriod" {
		t.Fatalf("agent.UsageSummary.Period type = %q, want UsageSummaryPeriod", got)
	}
	if _, exists := fields["SinceDays"]; exists {
		t.Fatal("agent.UsageSummary restored primitive SinceDays")
	}
	periodPath := filepath.Join(root, "internal", "domain", "agent", "usage_period.go")
	period := cliStructFieldTypes(t, periodPath, "UsageSummaryPeriod")
	if period["kind"] != "usageSummaryPeriodKind" || period["days"] != "int" {
		t.Fatalf("agent.UsageSummaryPeriod fields = %v, want private kind/days", period)
	}
}

func TestRuntimeBindingProfileOwnsRunConcurrencySemantics(t *testing.T) {
	root := moduleRoot(t)
	profilePath := filepath.Join(root, "internal", "adapter", "runtimebinding", "negotiated_profile.go")
	limits := cliStructFieldTypes(t, profilePath, "Limits")
	if got := limits["RunConcurrency"]; got != "RunConcurrencyLimit" {
		t.Fatalf("runtimebinding.Limits.RunConcurrency type = %q, want RunConcurrencyLimit", got)
	}
	if got := limits["CommandReplay"]; got != "commandreplay.Capability" {
		t.Fatalf("runtimebinding.Limits.CommandReplay type = %q, want commandreplay.Capability", got)
	}
	if _, exists := limits["MaxConcurrentRuns"]; exists {
		t.Fatal("runtimebinding.Limits restored primitive MaxConcurrentRuns")
	}
	for _, retired := range []string{"IdempotencyNamespace", "IdempotencyRetentionSeconds"} {
		if _, exists := limits[retired]; exists {
			t.Fatalf("runtimebinding.Limits restored primitive %s", retired)
		}
	}
	valuePath := filepath.Join(root, "internal", "adapter", "runtimebinding", "run_concurrency.go")
	value := cliStructFieldTypes(t, valuePath, "RunConcurrencyLimit")
	if value["kind"] != "RunConcurrencyLimitKind" || value["maximum"] != "int" {
		t.Fatalf("runtimebinding.RunConcurrencyLimit fields = %v, want private kind/maximum", value)
	}
}

func TestRuntimeBindingOwnsCommandReplayPolicyProjection(t *testing.T) {
	root := moduleRoot(t)
	path := filepath.Join(root, "internal", "adapter", "runtimebinding", "command_replay.go")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contents), "func CommandReplayPolicy(") ||
		!strings.Contains(string(contents), "func CommandReplayPolicyWithClock(") {
		t.Fatal("runtime binding does not own optional profile to replay policy projection")
	}
	for _, consumer := range []string{
		filepath.Join(root, "internal", "delivery", "terminal", "run.go"),
		filepath.Join(root, "internal", "delivery", "cmd", "run.go"),
		filepath.Join(root, "internal", "delivery", "cmd", "sessions.go"),
	} {
		contents, err := os.ReadFile(consumer)
		if err != nil {
			t.Fatal(err)
		}
		text := string(contents)
		if strings.Contains(text, "commandreplay.NewPolicy(") ||
			strings.Contains(text, "commandreplay.UnavailablePolicy(") {
			t.Errorf("%s restored a consumer-local replay policy projection", consumer)
		}
	}
}

func TestRuntimeConnectionDoesNotInferProfilePresenceFromBrandFields(t *testing.T) {
	root := moduleRoot(t)
	profilePath := filepath.Join(root, "internal", "adapter", "runtimebinding", "negotiated_profile.go")
	contents, err := os.ReadFile(profilePath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(contents), "func (p Profile) Available() bool") {
		t.Fatal("runtime profile restored Server.Name-based presence inference")
	}
	runtimePath := filepath.Join(root, "internal", "adapter", "runtimebinding", "connection.go")
	contents, err = os.ReadFile(runtimePath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(contents), ".profile.Available()") {
		t.Fatal("Runtime connection conditionally publishes its construction-owned profile")
	}
}

func TestWorkbenchOwnsExplicitPersistenceAndCapacitySemantics(t *testing.T) {
	root := moduleRoot(t)
	storePath := filepath.Join(root, "internal", "application", "agent", "workbench", "store.go")
	config := cliStructFieldTypes(t, storePath, "Config")
	for _, field := range []string{"HistoryCapacity", "StashCapacity", "WorkspaceCapacity"} {
		if got := config[field]; got != "*Capacity" {
			t.Errorf("workbench.Config.%s type = %q, want *Capacity", field, got)
		}
	}
	for _, retired := range []string{"HistoryLimit", "StashLimit", "WorkspaceLimit"} {
		if _, exists := config[retired]; exists {
			t.Errorf("workbench.Config restored primitive %s", retired)
		}
	}
	store := cliStructFieldTypes(t, storePath, "Store")
	if got := store["persistence"]; got != "persistence" {
		t.Fatalf("workbench.Store.persistence type = %q, want persistence", got)
	}
	if _, exists := store["directory"]; exists {
		t.Fatal("workbench.Store restored an empty-string persistence mode")
	}
	contents, err := os.ReadFile(storePath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(contents)
	for _, retired := range []string{"func Open(", "positiveOr(", "s.directory == \"\""} {
		if strings.Contains(text, retired) {
			t.Errorf("workbench Store restored retired construction semantics %q", retired)
		}
	}
}

func TestDurableCommandsShareOneReplayDomainModel(t *testing.T) {
	root := moduleRoot(t)
	checks := []struct {
		path      string
		structure string
		fields    []string
	}{
		{path: filepath.Join(root, "internal", "application", "agent", "workbench", "pending_run.go"), structure: "PendingRun", fields: []string{"Replay", "CancelReplay"}},
		{path: filepath.Join(root, "internal", "application", "agent", "workbench", "pending_run.go"), structure: "PendingResume", fields: []string{"Replay"}},
		{path: filepath.Join(root, "internal", "application", "agent", "workbench", "steer.go"), structure: "PendingSteer", fields: []string{"replay"}},
		{path: filepath.Join(root, "internal", "application", "agent", "workbench", "session_deletion.go"), structure: "PendingSessionDeletion", fields: []string{"Replay"}},
		{path: filepath.Join(root, "internal", "application", "agent", "workbench", "session_rollback.go"), structure: "PendingSessionRollback", fields: []string{"Replay"}},
	}
	for _, check := range checks {
		fields := cliStructFieldTypes(t, check.path, check.structure)
		for _, field := range check.fields {
			if got := fields[field]; got != "commandreplay.Guard" {
				t.Errorf("workbench.%s.%s type = %q, want commandreplay.Guard", check.structure, field, got)
			}
		}
		for _, retired := range []string{"ReplayNamespace", "ReplayUntil"} {
			if _, exists := fields[retired]; exists {
				t.Errorf("workbench.%s restored primitive %s", check.structure, retired)
			}
		}
	}
	invocation := cliStructFieldTypes(t, filepath.Join(root, "internal", "application", "agent", "run", "execution.go"), "Invocation")
	if got := invocation["ReplayPolicy"]; got != "commandreplay.Policy" {
		t.Fatalf("run.Invocation.ReplayPolicy type = %q, want commandreplay.Policy", got)
	}
	if _, exists := invocation["ReplayRetention"]; exists {
		t.Fatal("run.Invocation restored primitive ReplayRetention")
	}
	for _, path := range []string{
		filepath.Join(root, "internal", "application", "agent", "run", "steering.go"),
		filepath.Join(root, "internal", "application", "agent", "session", "deletion.go"),
		filepath.Join(root, "internal", "application", "agent", "session", "rollback.go"),
	} {
		contents, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(contents), "type ReplayWindow struct") {
			t.Errorf("%s restored a consumer-local ReplayWindow", path)
		}
	}
	mutationPath := filepath.Join(root, "internal", "application", "agent", "mutation", "confirmation.go")
	contents, err := os.ReadFile(mutationPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(contents)
	if !strings.Contains(text, "func FreshReplayAdmission(") ||
		!strings.Contains(text, "func ReplayAdmission(") {
		t.Fatal("mutation no longer distinguishes a fresh command attempt from durable replay")
	}
}

func TestPendingSteerSeparatesImmutableOwnershipFromPersistenceRecord(t *testing.T) {
	root := moduleRoot(t)
	path := filepath.Join(root, "internal", "application", "agent", "workbench", "steer.go")
	want := map[string]string{
		"sessionID": "string", "command": "agent.SteerRun", "stagedAt": "time.Time",
		"replay": "commandreplay.Guard",
	}
	if fields := cliStructFieldTypes(t, path, "PendingSteer"); !maps.Equal(fields, want) {
		t.Fatalf("workbench.PendingSteer fields = %v, want private ownership state %v", fields, want)
	}
	record := cliStructFieldTypes(t, path, "pendingSteerRecord")
	for field, fieldType := range map[string]string{
		"SessionID": "string", "Command": "agent.SteerRun", "StagedAt": "time.Time",
		"Replay": "commandreplay.Guard",
	} {
		if got := record[field]; got != fieldType {
			t.Errorf("workbench.pendingSteerRecord.%s type = %q, want %q", field, got, fieldType)
		}
	}

	applicationPath := filepath.Join(root, "internal", "application", "agent", "run", "steering.go")
	contents, err := os.ReadFile(applicationPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(contents)
	if !strings.Contains(text, "workbench.NewPendingSteer(") {
		t.Fatal("run StageSteer no longer constructs the durable aggregate through its owner")
	}
	if strings.Contains(text, "pending := workbench.PendingSteer{") {
		t.Fatal("run StageSteer restored caller-shaped pending steer construction")
	}
}

func TestTerminalOperationLeaseIdentityCannotWrap(t *testing.T) {
	root := moduleRoot(t)
	path := filepath.Join(root, "internal", "delivery", "terminal", "operations.go")
	if got := cliStructFieldTypes(t, path, "operationLease")["id"]; got != "operationLeaseID" {
		t.Fatalf("terminal.operationLease.id type = %q, want operationLeaseID", got)
	}
	if got := cliStructFieldTypes(t, path, "operationOwner")["next"]; got != "operationLeaseID" {
		t.Fatalf("terminal.operationOwner.next type = %q, want operationLeaseID", got)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(contents)
	for _, required := range []string{
		"func (id operationLeaseID) successor() (operationLeaseID, bool)",
		"next, available := o.next.successor()",
	} {
		if !strings.Contains(text, required) {
			t.Errorf("operation owner lost exhaustion boundary %q", required)
		}
	}
	if strings.Contains(text, "o.next++") {
		t.Fatal("operation owner restored unchecked lease identity increment")
	}
}

func TestStreamFollowerUsesSingleOperationOwnership(t *testing.T) {
	root := moduleRoot(t)
	path := filepath.Join(root, "internal", "delivery", "terminal", "run_stream.go")
	if got := cliStructFieldTypes(t, path, "streamFollower")["lease"]; got != "operationLease" {
		t.Fatalf("terminal.streamFollower.lease type = %q, want operationLease", got)
	}
	if _, exists := cliStructFieldTypes(t, filepath.Join(root, "internal", "delivery", "terminal", "application.go"), "app")["streamSeq"]; exists {
		t.Fatal("terminal app restored a parallel stream generation counter")
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(contents)
	for _, required := range []string{
		"func (s *streamFollower) current() bool",
		"s.app.operations.Current(s.lease)",
		"a.operations.Go(streamOperation, false, work)",
	} {
		if !strings.Contains(text, required) {
			t.Errorf("stream follower lost single-owner boundary %q", required)
		}
	}
	if strings.Contains(text, "streamSeq") {
		t.Fatal("stream follower restored a parallel numeric generation")
	}
}

func TestPluginCommandOperationRegistryOwnsCheckedIdentity(t *testing.T) {
	root := moduleRoot(t)
	applicationPath := filepath.Join(root, "internal", "delivery", "terminal", "application.go")
	if got := cliStructFieldTypes(t, applicationPath, "app")["commandOperations"]; got != "commandOperationRegistry" {
		t.Fatalf("terminal.app.commandOperations type = %q, want commandOperationRegistry", got)
	}
	path := filepath.Join(root, "internal", "delivery", "terminal", "commands.go")
	if got := cliStructFieldTypes(t, path, "commandOperationRegistry")["next"]; got != "commandOperationID" {
		t.Fatalf("terminal.commandOperationRegistry.next type = %q, want commandOperationID", got)
	}
	if got := cliStructFieldTypes(t, path, "commandOperation")["id"]; got != "commandOperationID" {
		t.Fatalf("terminal.commandOperation.id type = %q, want commandOperationID", got)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(contents)
	for _, required := range []string{
		"func (id commandOperationID) successor() (commandOperationID, bool)",
		"func (r *commandOperationRegistry) reserve(",
		"func (r *commandOperationRegistry) retire(",
		"func (r *commandOperationRegistry) take(",
		"pluginCommandOperationSlotPrefix",
	} {
		if !strings.Contains(text, required) {
			t.Errorf("plugin command registry lost ownership boundary %q", required)
		}
	}
	if strings.Contains(text, "commandSeq") {
		t.Fatal("plugin command registry restored unchecked primitive identity")
	}
}

func TestSessionPresentationUsesRetirableOwnership(t *testing.T) {
	root := moduleRoot(t)
	applicationPath := filepath.Join(root, "internal", "delivery", "terminal", "application.go")
	if got := cliStructFieldTypes(t, applicationPath, "app")["sessionContext"]; got != "*sessionContextLease" {
		t.Fatalf("terminal.app.sessionContext type = %q, want *sessionContextLease", got)
	}
	path := filepath.Join(root, "internal", "delivery", "terminal", "session_context.go")
	if fields := cliStructFieldTypes(t, path, "sessionContextLease"); !maps.Equal(fields, map[string]string{"retired": "bool"}) {
		t.Fatalf("terminal.sessionContextLease fields = %v, want private retirement state", fields)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(contents)
	for _, required := range []string{
		"func newSessionContextLease() *sessionContextLease",
		"func (s *sessionContextLease) retire()",
		"func (s *sessionContextLease) current(candidate *sessionContextLease) bool",
	} {
		if !strings.Contains(text, required) {
			t.Errorf("session context lost ownership boundary %q", required)
		}
	}
	if strings.Contains(text, "sessionContextEpoch") || strings.Contains(text, "advance()") {
		t.Fatal("session context restored a numeric epoch")
	}
}

func TestReusablePresentationsUseRetirableIdentity(t *testing.T) {
	root := moduleRoot(t)
	path := filepath.Join(root, "internal", "delivery", "terminal", "presentation_dialog.go")
	if got := cliStructFieldTypes(t, path, "presentationLease")["current"]; got != "*presentationIdentity" {
		t.Fatalf("terminal.presentationLease.current type = %q, want *presentationIdentity", got)
	}
	if fields := cliStructFieldTypes(t, path, "presentationIdentity"); !maps.Equal(fields, map[string]string{"retired": "bool"}) {
		t.Fatalf("terminal.presentationIdentity fields = %v, want private retirement state", fields)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(contents)
	for _, required := range []string{
		"presented headless.Snapshot[*presentationIdentity]",
		"func (p *presentationIdentity) retire()",
		"func (p *presentationIdentity) current(candidate *presentationIdentity) bool",
		"p.current = &presentationIdentity{}",
	} {
		if !strings.Contains(text, required) {
			t.Errorf("presentation lease lost ownership boundary %q", required)
		}
	}
	for _, retired := range []string{"p.current++", "presentation lease exhausted"} {
		if strings.Contains(text, retired) {
			t.Errorf("presentation lease restored numeric exhaustion path %q", retired)
		}
	}
}

func TestTranscriptFramesUseRetirableContentOwnership(t *testing.T) {
	root := moduleRoot(t)
	path := filepath.Join(root, "internal", "delivery", "terminal", "transcript.go")
	if got := cliStructFieldTypes(t, path, "transcriptView")["contentLease"]; got != "*transcriptContentLease" {
		t.Fatalf("terminal.transcriptView.contentLease type = %q, want *transcriptContentLease", got)
	}
	if got := cliStructFieldTypes(t, path, "transcriptBlockPresentation")["lease"]; got != "*transcriptContentLease" {
		t.Fatalf("terminal.transcriptBlockPresentation.lease type = %q, want *transcriptContentLease", got)
	}
	if fields := cliStructFieldTypes(t, path, "transcriptContentLease"); !maps.Equal(fields, map[string]string{"retired": "bool"}) {
		t.Fatalf("terminal.transcriptContentLease fields = %v, want private retirement state", fields)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(contents)
	for _, required := range []string{
		"func newTranscriptContentLease() *transcriptContentLease",
		"func (l *transcriptContentLease) retire()",
		"func (l *transcriptContentLease) current(candidate *transcriptContentLease) bool",
	} {
		if !strings.Contains(text, required) {
			t.Errorf("transcript content lost ownership boundary %q", required)
		}
	}
	if strings.Contains(text, "contentEpoch") {
		t.Fatal("transcript restored a numeric content epoch")
	}
}

func TestReaderDocumentObserversOwnNonReusableTokens(t *testing.T) {
	root := moduleRoot(t)
	observersPath := filepath.Join(root, "internal", "delivery", "terminal", "reader_document_observers.go")
	if fields := cliStructFieldTypes(t, observersPath, "readerDocumentObserverToken"); !maps.Equal(fields, map[string]string{"active": "bool"}) {
		t.Fatalf("terminal.readerDocumentObserverToken fields = %v, want private activity state", fields)
	}
	for _, aggregate := range []struct {
		path      string
		structure string
	}{
		{path: filepath.Join(root, "internal", "delivery", "terminal", "tool_block.go"), structure: "toolBlock"},
		{path: filepath.Join(root, "internal", "delivery", "terminal", "tool_group.go"), structure: "toolGroupBlock"},
	} {
		fields := cliStructFieldTypes(t, aggregate.path, aggregate.structure)
		if got := fields["observers"]; got != "readerDocumentObservers" {
			t.Errorf("terminal.%s.observers type = %q, want readerDocumentObservers", aggregate.structure, got)
		}
		if _, exists := fields["nextObserver"]; exists {
			t.Errorf("terminal.%s restored numeric observer identity", aggregate.structure)
		}
	}
	contents, err := os.ReadFile(observersPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(contents)
	for _, required := range []string{
		"subscriptions map[*readerDocumentObserverToken]func(readerDocument)",
		"func (o *readerDocumentObservers) observe(",
		"func (o *readerDocumentObservers) notify(",
		"token := &readerDocumentObserverToken{active: true}",
	} {
		if !strings.Contains(text, required) {
			t.Errorf("reader document observers lost ownership boundary %q", required)
		}
	}
}

func TestExtensionRegistryOwnsCheckedRegistrationSequence(t *testing.T) {
	root := moduleRoot(t)
	path := filepath.Join(root, "internal", "application", "extensions", "registry.go")
	if got := cliStructFieldTypes(t, path, "Registry")["next"]; got != "registrationSequence" {
		t.Fatalf("extensions.Registry.next type = %q, want registrationSequence", got)
	}
	if got := cliStructFieldTypes(t, path, "entry")["seq"]; got != "registrationSequence" {
		t.Fatalf("extensions.entry.seq type = %q, want registrationSequence", got)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(contents)
	for _, required := range []string{
		"plugins map[string]registrationSequence",
		"func (s registrationSequence) successor() (registrationSequence, bool)",
		"errRegistrationSequenceExhausted",
		"next, ok := r.next.successor()",
		"sequence, ok := r.next.successor()",
	} {
		if !strings.Contains(text, required) {
			t.Errorf("extension registry lost checked identity boundary %q", required)
		}
	}
	if strings.Contains(text, "r.next++") {
		t.Fatal("extension registry restored unchecked registration increment")
	}
}

func TestConversationDoesNotMaintainAnUnconsumedRevision(t *testing.T) {
	root := moduleRoot(t)
	path := filepath.Join(root, "internal", "domain", "agent", "conversation.go")
	if _, exists := cliStructFieldTypes(t, path, "Conversation")["revision"]; exists {
		t.Fatal("agent.Conversation restored an unconsumed revision counter")
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(contents)
	for _, required := range []string{
		"const conversationFailureBlockIDPrefix = \"failure:\"",
		"func (c *Conversation) appendFailureBlock(detail string)",
		"if _, exists := c.index[blockIdentity(c.runID, id)]; !exists",
	} {
		if !strings.Contains(text, required) {
			t.Errorf("conversation failure projection lost owned identity boundary %q", required)
		}
	}
	if strings.Contains(text, "c.revision") {
		t.Fatal("agent.Conversation restored mutation increments with no consumer")
	}
}

func TestRevisionedCLIProjectionsShareOneExactIntegerEnvelope(t *testing.T) {
	root := moduleRoot(t)
	counterPath := filepath.Join(root, "internal", "exactint", "counter.go")
	contents, err := os.ReadFile(counterPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(contents)
	for _, required := range []string{
		"const Maximum uint64 = 1<<53 - 1",
		"func Restore(value uint64) (Counter, error)",
		"func (c Counter) Advance(changes uint64) (Counter, error)",
		"func Follows(previous, next uint64) error",
	} {
		if !strings.Contains(text, required) {
			t.Errorf("exact integer owner lost %q", required)
		}
	}
	for _, consumer := range []string{
		filepath.Join(root, "internal", "domain", "agent", "session.go"),
		filepath.Join(root, "internal", "domain", "agent", "plan.go"),
		filepath.Join(root, "internal", "domain", "schedule", "schedule.go"),
	} {
		contents, err := os.ReadFile(consumer)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(contents), "exactint.") {
			t.Errorf("%s does not consume the exact revision owner", consumer)
		}
	}
}

func TestDraftPersistenceRevisionIdentityCannotWrap(t *testing.T) {
	root := moduleRoot(t)
	path := filepath.Join(root, "internal", "delivery", "terminal", "draft_persistence.go")
	if got := cliStructFieldTypes(t, path, "draftSnapshot")["revision"]; got != "draftRevision" {
		t.Fatalf("terminal.draftSnapshot.revision type = %q, want draftRevision", got)
	}
	if got := cliStructFieldTypes(t, path, "draftPersistence")["revision"]; got != "draftRevision" {
		t.Fatalf("terminal.draftPersistence.revision type = %q, want draftRevision", got)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(contents)
	for _, required := range []string{
		"func (r draftRevision) successor() (draftRevision, bool)",
		"errDraftPersistenceRevisionExhausted",
		"next, ok := d.revision.successor()",
	} {
		if !strings.Contains(text, required) {
			t.Errorf("draft persistence lost exhaustion boundary %q", required)
		}
	}
	if strings.Contains(text, "d.revision++") {
		t.Fatal("draft persistence restored unchecked revision increment")
	}
}

func TestCatalogQueriesOwnNamedPageSizeIntent(t *testing.T) {
	root := moduleRoot(t)
	agentRoot := filepath.Join(root, "internal", "domain", "agent")
	for _, check := range []struct {
		path      string
		structure string
	}{
		{path: filepath.Join(agentRoot, "session.go"), structure: "SessionQuery"},
		{path: filepath.Join(agentRoot, "run_catalog.go"), structure: "RunQuery"},
	} {
		fields := cliStructFieldTypes(t, check.path, check.structure)
		if got := fields["PageSize"]; got != "PageSize" {
			t.Errorf("agent.%s.PageSize type = %q, want PageSize", check.structure, got)
		}
		if _, exists := fields["Limit"]; exists {
			t.Errorf("agent.%s restored primitive Limit", check.structure)
		}
	}
	pageSize := cliStructFieldTypes(t, filepath.Join(agentRoot, "page_size.go"), "PageSize")
	if pageSize["kind"] != "pageSizeKind" || pageSize["rows"] != "int" {
		t.Fatalf("agent.PageSize fields = %v, want private kind/rows", pageSize)
	}
}

func TestRunLimitsRequireExplicitConstructionAndDurableIdentity(t *testing.T) {
	root := moduleRoot(t)
	path := filepath.Join(root, "internal", "domain", "agent", "run_limits.go")
	fields := cliStructFieldTypes(t, path, "RunLimits")
	if fields["initialized"] != "bool" {
		t.Fatalf("agent.RunLimits fields = %v, want explicit initialization state", fields)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(contents)
	for _, method := range []string{"func (r RunLimits) MarshalJSON()", "func (r *RunLimits) UnmarshalJSON("} {
		if !strings.Contains(text, method) {
			t.Errorf("agent.RunLimits lacks durable method %q", method)
		}
	}
	for _, consumer := range []struct {
		path string
		want string
	}{
		{path: filepath.Join(root, "internal", "application", "agent", "promptqueue", "queue.go"), want: "agent.UnlimitedRunLimits()"},
		{path: filepath.Join(root, "internal", "adapter", "runtimebinding", "projection.go"), want: "Limits: agent.UnlimitedRunLimits()"},
	} {
		contents, err := os.ReadFile(consumer.path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(contents), consumer.want) {
			t.Errorf("%s no longer constructs unlimited Run limits explicitly", consumer.path)
		}
	}
}

func TestRunLineageRequiresExplicitClosedConstruction(t *testing.T) {
	root := moduleRoot(t)
	path := filepath.Join(root, "internal", "domain", "agent", "run.go")
	fields := cliStructFieldTypes(t, path, "RunLineage")
	want := map[string]string{
		"kind": "runLineageKind", "spawnedByBlockID": "string",
		"parentRunID": "string", "rootRunID": "string",
	}
	if !maps.Equal(fields, want) {
		t.Fatalf("agent.RunLineage fields = %v, want private closed identity %v", fields, want)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(contents)
	for _, constructor := range []string{"func RootRunLineage()", "func NewChildRunLineage("} {
		if !strings.Contains(text, constructor) {
			t.Errorf("agent.RunLineage lacks constructor %q", constructor)
		}
	}
	if strings.Contains(text, "return r == (RunLineage{})") {
		t.Fatal("agent.RunLineage restored zero-value root inference")
	}
	projectionPath := filepath.Join(root, "internal", "adapter", "runtimebinding", "projection.go")
	contents, err = os.ReadFile(projectionPath)
	if err != nil {
		t.Fatal(err)
	}
	projection := string(contents)
	if !strings.Contains(projection, "agent.RootRunLineage()") || !strings.Contains(projection, "agent.NewChildRunLineage(") {
		t.Fatal("Runtime adapter no longer projects explicit root/child lineage")
	}
}

func TestModelRolesOwnExplicitInheritedDisabledAndConfiguredModes(t *testing.T) {
	root := moduleRoot(t)
	path := filepath.Join(root, "internal", "application", "integration", "models", "configuration.go")
	fields := cliStructFieldTypes(t, path, "Role")
	want := map[string]string{
		"kind": "RoleKind", "mode": "roleMode", "provider": "string", "model": "string",
	}
	if !maps.Equal(fields, want) {
		t.Fatalf("models.Role fields = %v, want private closed role %v", fields, want)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(contents)
	for _, constructor := range []string{
		"func InheritedUtilityRole()", "func DisabledEmbeddingRole()", "func NewConfiguredRole(",
	} {
		if !strings.Contains(text, constructor) {
			t.Errorf("models.Role lacks constructor %q", constructor)
		}
	}
	projectionPath := filepath.Join(root, "internal", "adapter", "runtimebinding", "models.go")
	contents, err = os.ReadFile(projectionPath)
	if err != nil {
		t.Fatal(err)
	}
	projection := string(contents)
	if !strings.Contains(projection, "projectUtilityRole(") || !strings.Contains(projection, "projectEmbeddingRole(") {
		t.Fatal("Runtime adapter no longer owns role-specific wire projection")
	}
	terminalPath := filepath.Join(root, "internal", "delivery", "terminal", "runtime_management.go")
	contents, err = os.ReadFile(terminalPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(contents), "models.Role{Kind:") {
		t.Fatal("terminal restored caller-shaped model role construction")
	}
}

func TestPromptQueueIdentityAndDispatchPresenceHaveNoNumericSentinel(t *testing.T) {
	root := moduleRoot(t)
	queuePath := filepath.Join(root, "internal", "application", "agent", "promptqueue", "queue.go")
	entry := cliStructFieldTypes(t, queuePath, "Entry")
	if got := entry["ID"]; got != "EntryID" {
		t.Fatalf("promptqueue.Entry.ID type = %q, want EntryID", got)
	}
	state := cliStructFieldTypes(t, queuePath, "State")
	if got := state["Dispatching"]; got != "*EntryID" {
		t.Fatalf("promptqueue.State.Dispatching type = %q, want *EntryID", got)
	}
	if _, exists := state["DispatchingID"]; exists {
		t.Fatal("promptqueue.State restored scalar dispatch sentinel")
	}
	if _, exists := cliStructFieldTypes(t, queuePath, "Snapshot")["Revision"]; exists {
		t.Fatal("promptqueue.Snapshot restored an unconsumed revision")
	}
	if _, exists := cliStructFieldTypes(t, queuePath, "Queue")["revision"]; exists {
		t.Fatal("promptqueue.Queue restored an unowned revision counter")
	}
	idPath := filepath.Join(root, "internal", "application", "agent", "promptqueue", "entry_id.go")
	id := cliStructFieldTypes(t, idPath, "EntryID")
	if !maps.Equal(id, map[string]string{"value": "uint64"}) {
		t.Fatalf("promptqueue.EntryID fields = %v, want private uint64 value", id)
	}
	contents, err := os.ReadFile(queuePath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(contents)
	for _, retired := range []string{"map[string]uint64", "DispatchingID uint64", "id uint64"} {
		if strings.Contains(text, retired) {
			t.Errorf("prompt queue restored numeric identity sentinel shape %q", retired)
		}
	}
	drawerPath := filepath.Join(root, "internal", "delivery", "terminal", "queue_drawer.go")
	drawer := cliStructFieldTypes(t, drawerPath, "queueDrawer")
	if drawer["selectedID"] != "*promptqueue.EntryID" || drawer["editingEntry"] != "*promptqueue.Entry" {
		t.Fatalf("terminal.queueDrawer identity state = %v, want explicit optional selection/editing", drawer)
	}
}

func TestChangefeedSequenceWatermarkCannotRegressOrWrap(t *testing.T) {
	root := moduleRoot(t)
	path := filepath.Join(root, "internal", "application", "changefeed", "sequence.go")
	tracker := cliStructFieldTypes(t, path, "SequenceTracker")
	if !maps.Equal(tracker, map[string]string{"last": "*sequence"}) {
		t.Fatalf("changefeed.SequenceTracker fields = %v, want optional rich watermark", tracker)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(contents)
	for _, disposition := range []string{"SequenceContiguous", "SequenceGap", "SequenceStale"} {
		if !strings.Contains(text, disposition) {
			t.Errorf("changefeed sequence tracker lacks %s", disposition)
		}
	}
	consumerPath := filepath.Join(root, "internal", "delivery", "terminal", "workspace_inspection.go")
	contents, err = os.ReadFile(consumerPath)
	if err != nil {
		t.Fatal(err)
	}
	consumer := string(contents)
	if !strings.Contains(consumer, "changefeed.NewSequenceTracker()") || strings.Contains(consumer, "*lastSequence+1") {
		t.Fatal("runtime change monitor restored primitive sequence arithmetic")
	}
}

func TestRetrySchedulesRequireNamedConstructionAndOneTerminalOwner(t *testing.T) {
	root := moduleRoot(t)
	retryPath := filepath.Join(root, "internal", "application", "retry", "retry.go")
	backoff := cliStructFieldTypes(t, retryPath, "Backoff")
	if backoff["mode"] != "backoffMode" || backoff["base"] != "time.Duration" || backoff["maximum"] != "time.Duration" {
		t.Fatalf("retry.Backoff fields = %v, want private mode/base/maximum", backoff)
	}
	for _, leaked := range []string{"Base", "Maximum"} {
		if _, exists := backoff[leaked]; exists {
			t.Errorf("retry.Backoff restored mutable exported field %s", leaked)
		}
	}
	reconnectPath := filepath.Join(root, "internal", "application", "retry", "reconnect.go")
	policy := cliStructFieldTypes(t, reconnectPath, "ReconnectPolicy")
	if len(policy) != 2 || policy["attempts"] != "int" || policy["backoff"] != "Backoff" {
		t.Fatalf("retry.ReconnectPolicy fields = %v, want only private attempt budget and shared backoff", policy)
	}
	applicationPath := filepath.Join(root, "internal", "delivery", "terminal", "application.go")
	if got := cliStructFieldTypes(t, applicationPath, "app")["reconnectPolicy"]; got != "retry.ReconnectPolicy" {
		t.Fatalf("terminal.app.reconnectPolicy type = %q, want retry.ReconnectPolicy", got)
	}
}

func TestAttachmentCompletionOwnsItsFiniteResultBudget(t *testing.T) {
	root := moduleRoot(t)
	path := filepath.Join(root, "internal", "adapter", "filesystem", "attachment", "resolver.go")
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatalf("parse attachment resolver: %v", err)
	}
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Name.Name != "Complete" || function.Recv == nil {
			continue
		}
		parameters := 0
		for _, field := range function.Type.Params.List {
			parameters += max(1, len(field.Names))
		}
		if parameters != 2 {
			t.Fatalf("attachment.Resolver.Complete parameters = %d, want context and query only", parameters)
		}
		return
	}
	t.Fatal("attachment.Resolver.Complete method is missing")
}

func TestPromptHistoryOwnsOneNamedRetentionCapacity(t *testing.T) {
	root := moduleRoot(t)
	path := filepath.Join(root, "internal", "delivery", "terminal", "composer.go")
	fields := cliStructFieldTypes(t, path, "promptHistory")
	if _, exists := fields["limit"]; exists {
		t.Fatal("terminal.promptHistory restored caller-shaped limit field")
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(contents)
	if !strings.Contains(text, "const promptHistoryCapacity = 1000") ||
		strings.Contains(text, "if limit <= 0") {
		t.Fatal("terminal.promptHistory does not own one named capacity")
	}
}

func TestSideloadCommandTimeoutKeepsJSONPresenceInARichDeclaration(t *testing.T) {
	root := moduleRoot(t)
	path := filepath.Join(root, "internal", "delivery", "terminal", "sideload", "directory_source.go")
	manifest := cliStructFieldTypes(t, path, "commandManifest")
	if got := manifest["Timeout"]; got != "commandTimeoutDeclaration" {
		t.Fatalf("sideload.commandManifest.Timeout type = %q, want commandTimeoutDeclaration", got)
	}
	if _, exists := manifest["TimeoutSeconds"]; exists {
		t.Fatal("sideload.commandManifest restored primitive TimeoutSeconds")
	}
	declaration := cliStructFieldTypes(t, path, "commandTimeoutDeclaration")
	if declaration["present"] != "bool" || declaration["seconds"] != "int" {
		t.Fatalf("sideload.commandTimeoutDeclaration fields = %v, want private present/seconds", declaration)
	}
}

func cliStructFieldTypes(t *testing.T, path, structure string) map[string]string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	for _, declaration := range file.Decls {
		generic, ok := declaration.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, specification := range generic.Specs {
			named, ok := specification.(*ast.TypeSpec)
			if !ok || named.Name.Name != structure {
				continue
			}
			value, ok := named.Type.(*ast.StructType)
			if !ok {
				t.Fatalf("%s.%s is not a struct", path, structure)
			}
			fields := make(map[string]string)
			for _, field := range value.Fields.List {
				typeName := cliTypeName(field.Type)
				if typeName == "" {
					continue
				}
				for _, name := range field.Names {
					fields[name.Name] = typeName
				}
			}
			return fields
		}
	}
	t.Fatalf("%s has no struct %s", path, structure)
	return nil
}

func cliTypeName(expression ast.Expr) string {
	switch value := expression.(type) {
	case *ast.Ident:
		return value.Name
	case *ast.SelectorExpr:
		owner := cliTypeName(value.X)
		if owner == "" {
			return ""
		}
		return owner + "." + value.Sel.Name
	case *ast.StarExpr:
		return "*" + cliTypeName(value.X)
	case *ast.ArrayType:
		return "[]" + cliTypeName(value.Elt)
	default:
		return ""
	}
}

const (
	modulePath  = "github.com/Tangerg/flame/cli"
	libraryPath = "github.com/Tangerg/oolong"
	runtimePath = "github.com/Tangerg/flame/runtime"
	cobraPath   = "github.com/spf13/cobra"
	viperPath   = "github.com/spf13/viper"
)

// The layers, longest prefix first so the first match wins.
var layers = []struct {
	prefix string
	name   string
}{
	{"internal/testsupport/runtimefixture/", "runtimefixture"},
	{"internal/exactint/", "exactint"},
	{"internal/domain/identity/", "identity"},
	{"internal/adapter/runtimebinding/", "runtimebinding"},
	{"internal/domain/failure/", "failure"},
	{"internal/application/changefeed/", "changefeed"},
	{"internal/domain/workspace/", "workspace"},
	{"internal/application/integration/models/", "models"},
	{"internal/application/integration/mcp/", "mcp"},
	{"internal/domain/schedule/", "schedule"},
	{"internal/delivery/terminal/sideload/", "sideload"},
	{"internal/delivery/terminal/", "terminal"},
	{"internal/adapter/filesystem/attachment/", "attachment"},
	{"internal/application/agent/promptqueue/", "promptqueue"},
	{"internal/application/agent/mutation/", "mutation"},
	{"internal/application/retry/", "retry"},
	{"internal/application/agent/run/", "run"},
	{"internal/domain/commandreplay/", "commandreplay"},
	{"internal/application/agent/session/", "session"},
	{"internal/adapter/filesystem/sessionartifact/", "sessionartifact"},
	{"internal/application/agent/workbench/", "workbench"},
	{"internal/domain/agent/", "agent"},
	{"internal/application/settings/", "settings"},
	{"internal/application/extensions/", "extensions"},
	{"internal/delivery/cmd/render/", "render"},
	{"internal/delivery/cmd/", "cmd"},
	{"internal/arch/", "arch"},
}

// allowed names every inward or same-ring dependency. An allowlist makes a new
// dependency fail closed instead of silently weakening the architecture.
var allowed = map[string][]string{
	// Domain policy and generic infrastructure are the center.
	"failure":         {"identity"},
	"exactint":        nil,
	"identity":        nil,
	"commandreplay":   nil,
	"agent":           {"exactint", "failure", "identity", "workspace"},
	"changefeed":      nil,
	"workspace":       nil,
	"models":          {"failure", "identity"},
	"mcp":             {"failure"},
	"schedule":        {"exactint", "identity"},
	"settings":        {"agent"},
	"session":         {"agent", "commandreplay", "identity", "mutation", "retry", "workbench"},
	"run":             {"agent", "commandreplay", "mutation", "retry", "workbench"},
	"mutation":        {"agent", "commandreplay", "retry"},
	"retry":           {"agent"},
	"extensions":      nil,
	"promptqueue":     {"agent", "identity"},
	"sessionartifact": {"session"},
	"workbench":       {"agent", "commandreplay", "identity"},

	// Outbound adapters share domain contracts, not one another.
	"attachment":     {"agent"},
	"runtimefixture": {"agent", "exactint", "failure", "identity", "workspace"},
	"runtimebinding": {"agent", "changefeed", "commandreplay", "failure", "identity", "mcp", "models", "schedule", "session", "workspace"},
	"render":         {"agent", "failure", "identity"},

	// Delivery adapters consume inward abstractions. Sideloading is the outer trust
	// boundary around terminal contributions; main is the only composition root.
	"terminal": {"agent", "attachment", "changefeed", "commandreplay", "extensions", "failure", "identity", "mcp", "models", "mutation", "promptqueue", "retry", "run", "runtimebinding", "schedule", "session", "sessionartifact", "settings", "workbench", "workspace"},
	"sideload": {"extensions", "terminal"},
	"cmd":      {"agent", "attachment", "commandreplay", "failure", "mutation", "render", "run", "runtimebinding", "session", "settings", "workbench"},
	"arch":     nil,
}

var adapterDependencies = []struct {
	path          string
	allowedLayers []string
}{
	{path: runtimePath, allowedLayers: []string{"runtimebinding"}},
	{path: cobraPath, allowedLayers: []string{"cmd"}},
	{path: viperPath, allowedLayers: []string{"cmd"}},
}

func TestLayeringHoldsInTheImports(t *testing.T) {
	root := moduleRoot(t)
	fset := token.NewFileSet()

	checked := 0
	walk(t, root, func(dir, path string) {
		from := layerOf(dir)
		if from == "" {
			return
		}
		checked++
		for _, imported := range imports(t, fset, path) {
			rest, ok := strings.CutPrefix(imported, modulePath+"/")
			if !ok {
				continue
			}
			to := layerOf(rest)
			if to == "" || to == from {
				continue
			}
			if !slices.Contains(allowed[from], to) {
				t.Errorf("%s (%s) imports %s (%s): %s must never depend on %s",
					dir, from, rest, to, from, to)
			}
		}
	})
	if checked == 0 {
		t.Fatal("no files were checked, so this test proves nothing")
	}
}

func TestEveryInternalPackageBelongsToALayer(t *testing.T) {
	root := moduleRoot(t)
	seen := make(map[string]struct{})
	walk(t, root, func(dir, _ string) {
		if !strings.HasPrefix(dir, "internal/") || layerOf(dir) != "" {
			return
		}
		seen[dir] = struct{}{}
	})
	for _, dir := range slices.Sorted(maps.Keys(seen)) {
		t.Errorf("%s belongs to no architecture layer", dir)
	}
}

// TestRingPackagesUseContextHierarchy keeps a ring from regressing into a flat
// catalog. Direct packages must be ring-wide mechanisms or context-root
// aggregates; related capability families use one non-package namespace.
func TestRingPackagesUseContextHierarchy(t *testing.T) {
	topologies := []struct {
		ring     string
		direct   []string
		contexts []string
	}{
		{
			ring:   "domain",
			direct: []string{"agent", "commandreplay", "failure", "identity", "schedule", "workspace"},
		},
		{
			ring:     "application",
			direct:   []string{"changefeed", "extensions", "retry", "settings"},
			contexts: []string{"agent", "integration"},
		},
		{
			ring:     "adapter",
			direct:   []string{"runtimebinding"},
			contexts: []string{"filesystem"},
		},
		{
			ring:     "delivery",
			direct:   []string{"cmd", "terminal"},
			contexts: []string{"cmd", "terminal"},
		},
	}

	root := moduleRoot(t)
	packages := make(map[string]struct{})
	walk(t, root, func(dir, _ string) {
		if strings.HasPrefix(dir, "internal/") {
			packages[dir] = struct{}{}
		}
	})
	for _, topology := range topologies {
		t.Run(topology.ring, func(t *testing.T) {
			prefix := "internal/" + topology.ring + "/"
			for dir := range packages {
				if !strings.HasPrefix(dir, prefix) {
					continue
				}
				relative := strings.TrimPrefix(dir, prefix)
				parts := strings.Split(relative, "/")
				switch len(parts) {
				case 1:
					if !slices.Contains(topology.direct, relative) {
						t.Errorf("%s is a flat package without ring-wide ownership; place it under a proven context namespace", relative)
					}
				case 2:
					if !slices.Contains(topology.contexts, parts[0]) {
						t.Errorf("%s uses unreviewed context namespace %s", relative, parts[0])
					}
				default:
					t.Errorf("%s nests beyond ring/context/package", relative)
				}
			}
		})
	}
}

func TestPackagePathsDoNotRepeatTheirOwners(t *testing.T) {
	root := moduleRoot(t)
	packages := make(map[string]struct{})
	walk(t, root, func(dir, _ string) {
		if strings.HasPrefix(dir, "internal/") {
			packages[dir] = struct{}{}
		}
	})
	for _, dir := range slices.Sorted(maps.Keys(packages)) {
		parts := strings.Split(strings.TrimPrefix(dir, "internal/"), "/")
		leaf := parts[len(parts)-1]
		for _, owner := range parts[:len(parts)-1] {
			if len(leaf) > len(owner) &&
				(strings.HasPrefix(leaf, owner) || strings.HasSuffix(leaf, owner)) {
				t.Errorf("%s repeats enclosing owner %s; let the import path supply that vocabulary", dir, owner)
			}
		}
	}
}

// TestTheLibraryStaysALibrary is the rule that keeps the interface library extractable.
//
// It is in its own module, so a reference from the library back into this program will
// not compile — that much is guaranteed by Go. What this test guards is the other
// direction: which of this program's layers are allowed to know that a terminal exists
// at all. The data and the renderers must not, or the interface stops being one choice
// among several and becomes the only one.
func TestTheLibraryStaysALibrary(t *testing.T) {
	root := moduleRoot(t)
	fset := token.NewFileSet()

	terminalFree := []string{"agent", "changefeed", "commandreplay", "exactint", "failure", "identity", "mcp", "models", "mutation", "retry", "runtimebinding", "schedule", "workspace", "settings", "runtimefixture", "attachment", "promptqueue", "run", "session", "sessionartifact", "workbench", "extensions", "render"}
	walk(t, root, func(dir, path string) {
		layer := layerOf(dir)
		if !slices.Contains(terminalFree, layer) {
			return
		}
		for _, imported := range imports(t, fset, path) {
			if strings.HasPrefix(imported, libraryPath) {
				t.Errorf("%s (%s) imports %s: this layer must not know there is a terminal",
					dir, layer, imported)
			}
		}
	})
}

func TestAdapterDependenciesStayAtTheirBoundaries(t *testing.T) {
	root := moduleRoot(t)
	fset := token.NewFileSet()
	walk(t, root, func(dir, path string) {
		layer := layerOf(dir)
		if layer == "" {
			return
		}
		for _, imported := range imports(t, fset, path) {
			for _, dependency := range adapterDependencies {
				if importsPath(imported, dependency.path) && !slices.Contains(dependency.allowedLayers, layer) {
					t.Errorf("%s (%s) imports %s: %s is confined to %s",
						dir, layer, imported, dependency.path, strings.Join(dependency.allowedLayers, ", "))
				}
			}
		}
	})
}

func TestAdapterBoundaryRulesRefuseInwardLeaks(t *testing.T) {
	for _, test := range []struct {
		layer, imported string
	}{
		{layer: "agent", imported: runtimePath + "/protocol"},
		{layer: "settings", imported: cobraPath},
		{layer: "run", imported: viperPath},
	} {
		if adapterDependencyAllowed(test.layer, test.imported) {
			t.Errorf("%s unexpectedly accepts %s", test.layer, test.imported)
		}
	}
}

func adapterDependencyAllowed(layer, imported string) bool {
	for _, dependency := range adapterDependencies {
		if importsPath(imported, dependency.path) {
			return slices.Contains(dependency.allowedLayers, layer)
		}
	}
	return true
}

func importsPath(imported, dependency string) bool {
	return imported == dependency || strings.HasPrefix(imported, dependency+"/")
}

// TestTheRulesWouldActuallyRefuseSomething is the counter-example: a guard never shown
// to fail is a guard nobody knows is wired up.
func TestTheRulesWouldActuallyRefuseSomething(t *testing.T) {
	for _, tc := range []struct {
		from, to string
		refused  bool
	}{
		{"internal/domain/agent", "internal/testsupport/runtimefixture", true},
		{"internal/domain/agent", "internal/delivery/terminal", true},
		{"internal/domain/agent", "internal/adapter/runtimebinding", true},
		{"internal/application/extensions", "internal/domain/agent", true},
		{"internal/testsupport/runtimefixture", "internal/delivery/cmd/render", true},
		{"internal/adapter/filesystem/attachment", "internal/delivery/terminal", true},
		{"internal/application/retry", "internal/delivery/cmd", true},
		{"internal/application/agent/run", "internal/delivery/cmd", true},
		{"internal/application/agent/session", "internal/delivery/terminal", true},
		{"internal/adapter/filesystem/sessionartifact", "internal/delivery/terminal", true},
		{"internal/application/agent/workbench", "internal/delivery/terminal", true},
		{"internal/application/settings", "internal/delivery/terminal", true},
		{"internal/application/agent/promptqueue", "internal/delivery/terminal", true},
		{"internal/delivery/cmd/render", "internal/delivery/terminal", true},
		{"internal/delivery/terminal", "internal/delivery/cmd", true},
		{"internal/delivery/terminal/sideload", "internal/delivery/cmd", true},

		{"internal/testsupport/runtimefixture", "internal/domain/agent", false},
		{"internal/adapter/runtimebinding", "internal/domain/agent", false},
		{"internal/adapter/runtimebinding", "internal/delivery/terminal", true},
		{"internal/delivery/terminal", "internal/domain/agent", false},
		{"internal/delivery/terminal", "internal/adapter/filesystem/sessionartifact", false},
		{"internal/delivery/terminal", "internal/application/agent/session", false},
		{"internal/delivery/terminal", "internal/application/agent/workbench", false},
		{"internal/delivery/terminal", "internal/application/extensions", false},
		{"internal/delivery/cmd", "internal/delivery/terminal", true},
		{"internal/delivery/terminal/sideload", "internal/application/extensions", false},
		{"internal/delivery/cmd/render", "internal/domain/agent", false},
		{"internal/adapter/filesystem/attachment", "internal/domain/agent", false},
		{"internal/application/retry", "internal/domain/agent", false},
		{"internal/application/agent/run", "internal/domain/agent", false},
		{"internal/delivery/cmd", "internal/application/agent/session", false},
		{"internal/delivery/cmd", "internal/application/agent/run", false},
		{"internal/application/settings", "internal/domain/agent", false},
		{"internal/application/agent/session", "internal/domain/agent", false},
		{"internal/application/agent/promptqueue", "internal/domain/agent", false},
	} {
		from, to := layerOf(tc.from), layerOf(tc.to)
		if from == "" {
			t.Fatalf("%s belongs to no layer, so nothing about it is guarded", tc.from)
		}
		if to == "" {
			t.Fatalf("%s belongs to no layer, so importing it is unguarded", tc.to)
		}
		got := from != to && !slices.Contains(allowed[from], to)
		if got != tc.refused {
			verb := map[bool]string{true: "refused", false: "allowed"}
			t.Errorf("%s -> %s is %s, want it %s", from, to, verb[got], verb[tc.refused])
		}
	}
}

// layerOf names the layer a module-relative directory belongs to.
func layerOf(dir string) string {
	dir = filepath.ToSlash(dir)
	if !strings.HasSuffix(dir, "/") {
		dir += "/"
	}
	for _, l := range layers {
		if strings.HasPrefix(dir, l.prefix) {
			return l.name
		}
	}
	return ""
}

// walk visits every production Go file, reporting its directory relative to the
// module root. Test files are skipped: a test may reach across layers for a
// fixture, and constraining that would buy nothing.
func walk(t *testing.T, root string, visit func(dir, path string)) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if name := d.Name(); name == "vendor" || (strings.HasPrefix(name, ".") && path != root) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		dir, err := filepath.Rel(root, filepath.Dir(path))
		if err != nil {
			return err
		}
		visit(filepath.ToSlash(dir), path)
		return nil
	})
	if err != nil {
		t.Fatalf("walk the module: %v", err)
	}
}

// imports reads one file's import paths.
func imports(t *testing.T, fset *token.FileSet, path string) []string {
	t.Helper()
	file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	out := make([]string, 0, len(file.Imports))
	for _, spec := range file.Imports {
		out = append(out, strings.Trim(spec.Path.Value, `"`))
	}
	return out
}

// moduleRoot is the directory holding go.mod.
func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no go.mod above the working directory")
		}
		dir = parent
	}
}
