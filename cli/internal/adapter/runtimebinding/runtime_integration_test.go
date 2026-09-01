package runtimebinding

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/protocol"

	"github.com/Tangerg/flame/cli/internal/application/agent/session"
	"github.com/Tangerg/flame/cli/internal/application/changefeed"
	"github.com/Tangerg/flame/cli/internal/application/integration/mcp"
	"github.com/Tangerg/flame/cli/internal/application/integration/models"
	"github.com/Tangerg/flame/cli/internal/domain/agent"
	workspaceapi "github.com/Tangerg/flame/cli/internal/domain/workspace"
)

func TestRuntimeConnectionSessionCatalogAndLifecycle(t *testing.T) {
	configureIntegrationRuntime(t)
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "main.go"), []byte("package main\n\nvar answer = 42\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(workspace, "empty"), 0o700); err != nil {
		t.Fatal(err)
	}
	runtime := openIntegrationRuntime(t, workspace)
	created := requireSessionCatalog(t, runtime, workspace)
	requireWorkspaceInspection(t, runtime, workspace)
	forked := requireSessionMutation(t, runtime, created, t.TempDir())
	requireSessionPortability(t, runtime, forked.ID)
	requireRuntimeCatalogs(t, runtime, created.ID, created.Workspace.Path)
	requireProviderMutationLifecycle(t, runtime)
	requireGoalMutationLifecycle(t, runtime, created.ID)
	requireContextManagement(t, runtime, created.Workspace.Path)
	requireAuxiliaryCapabilities(t, runtime, created.ID, created.Workspace.Path)
	requireExternalAuthoredInvalidations(t, runtime, created.Workspace.Path)
	requireSessionDeletion(t, runtime, created.ID, forked.ID)
	requireClosedRuntime(t, runtime)
}

func requireGoalMutationLifecycle(t *testing.T, runtime *Connection, sessionID string) {
	t.Helper()
	start := protocol.StartGoalRequest{
		SessionID: sessionID, Objective: "verify embedded goal lifecycle",
		Provider: "missing", Model: "missing", Budget: limitedGoalBudget(t, 3),
	}
	started, err := runtime.StartGoal(t.Context(), start)
	if err != nil {
		t.Fatalf("StartGoal: %v", err)
	}
	if started.Status != protocol.GoalActive || started.Objective != start.Objective {
		t.Fatalf("started goal: %+v", started)
	}
	update := protocol.UpdateGoalRequest{SessionID: sessionID, Objective: "verify revised embedded goal lifecycle"}
	updated, err := runtime.UpdateGoal(t.Context(), update)
	if err != nil {
		t.Fatalf("UpdateGoal: %v", err)
	}
	if updated.Objective != update.Objective {
		t.Fatalf("updated goal: %+v", updated)
	}
	stopped, err := runtime.StopGoal(t.Context(), sessionID)
	if err != nil {
		t.Fatalf("StopGoal: %v", err)
	}
	if stopped.Status == protocol.GoalActive {
		t.Fatalf("stopped goal remained active: %+v", stopped)
	}
	resumed, err := runtime.ResumeGoal(t.Context(), sessionID)
	if err != nil {
		t.Fatalf("ResumeGoal: %v", err)
	}
	if resumed.Status != protocol.GoalActive {
		t.Fatalf("resumed goal = %+v", resumed)
	}
	if _, err := runtime.StopGoal(t.Context(), sessionID); err != nil {
		t.Fatalf("final StopGoal: %v", err)
	}
	if err := runtime.ClearGoal(t.Context(), sessionID); err != nil {
		t.Fatalf("ClearGoal: %v", err)
	}
	if _, exists, err := runtime.GetGoal(t.Context(), sessionID); err != nil || exists {
		t.Fatalf("goal after clear = (exists=%t, err=%v)", exists, err)
	}
}

func requireExternalAuthoredInvalidations(t *testing.T, runtime *Connection, workspace string) {
	t.Helper()
	for _, topic := range []changefeed.Topic{changefeed.KnowledgeChanged, changefeed.HooksChanged} {
		if !runtime.Supports(topic) {
			t.Fatalf("embedded runtime did not advertise %s", topic)
		}
	}

	streamContext, cancelStream := context.WithCancel(t.Context())
	stream, err := runtime.Subscribe(streamContext, changefeed.Subscription{
		Topics: []changefeed.Topic{
			changefeed.FilesChanged,
			changefeed.KnowledgeChanged,
			changefeed.HooksChanged,
		},
		Watches: []changefeed.Watch{{ID: "authored-resources", Workspace: workspace}},
	})
	if err != nil {
		t.Fatalf("subscribe to authored resources: %v", err)
	}
	events := make(chan changefeed.Event, 8)
	streamErrors := make(chan error, 1)
	streamStopped := make(chan struct{})
	go func() {
		defer close(streamStopped)
		for event, streamErr := range stream {
			if streamErr != nil {
				streamErrors <- streamErr
				return
			}
			select {
			case events <- event:
			case <-streamContext.Done():
				return
			}
		}
	}()
	defer func() {
		cancelStream()
		select {
		case <-streamStopped:
		case <-time.After(3 * time.Second):
			t.Error("authored-resource subscription did not stop")
		}
	}()

	knowledgePath := filepath.Join(workspace, "FLAME.md")
	if writeFileErr := os.WriteFile(knowledgePath, []byte("# External knowledge\n"), 0o600); writeFileErr != nil {
		t.Fatalf("write external knowledge: %v", writeFileErr)
	}
	awaitRuntimeInvalidation(t, events, streamErrors, changefeed.KnowledgeChanged)
	target, err := workspaceapi.NewKnowledgeTarget(workspaceapi.KnowledgeWorkingDirectory, workspace)
	if err != nil {
		t.Fatal(err)
	}
	document, err := runtime.Knowledge().Document(t.Context(), target)
	if err != nil || document.Content != "# External knowledge\n" {
		t.Fatalf("knowledge after external invalidation = (%+v, %v)", document, err)
	}

	hooksDirectory := filepath.Join(workspace, ".flame")
	if mkdirAllErr := os.MkdirAll(hooksDirectory, 0o700); mkdirAllErr != nil {
		t.Fatalf("create external hooks directory: %v", mkdirAllErr)
	}
	if writeFileErr := os.WriteFile(
		filepath.Join(hooksDirectory, "hooks.json"),
		[]byte(`{"hooks":[{"event":"SessionStart","inject":"external context"}]}`),
		0o600,
	); writeFileErr != nil {
		t.Fatalf("write external hooks: %v", writeFileErr)
	}
	awaitRuntimeInvalidation(t, events, streamErrors, changefeed.HooksChanged)
	catalog, err := runtime.Hooks().Catalog(t.Context(), workspace)
	if err != nil || len(catalog.Hooks) != 1 || catalog.Hooks[0].Inject != "external context" {
		t.Fatalf("hooks after external invalidation = (%+v, %v)", catalog, err)
	}
}

func awaitRuntimeInvalidation(
	t *testing.T,
	events <-chan changefeed.Event,
	streamErrors <-chan error,
	topic changefeed.Topic,
) {
	t.Helper()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for {
		select {
		case event := <-events:
			if event.Type == changefeed.EventType(topic) {
				return
			}
		case err := <-streamErrors:
			t.Fatalf("wait for %s: %v", topic, err)
		case <-timer.C:
			t.Fatalf("no %s invalidation after external edit", topic)
		}
	}
}

func requireAuxiliaryCapabilities(t *testing.T, runtime *Connection, sessionID, workspace string) {
	t.Helper()
	diagnosticTools := runtime.DiagnosticTools()
	authoringContext := runtime.AuthoringContext()
	hooks := runtime.Hooks()
	feedbackService := runtime.Feedback()
	if diagnosticTools == nil || authoringContext == nil || hooks == nil || feedbackService == nil {
		t.Fatal("stable auxiliary adapters were not constructed")
	}
	tools, err := diagnosticTools.Tools(t.Context())
	if err != nil || len(tools) == 0 {
		t.Fatalf("DiagnosticTools = (%+v, %v)", tools, err)
	}
	if documents, err := authoringContext.Documents(t.Context(), workspace); err != nil {
		t.Fatalf("Agent documents = (%+v, %v)", documents, err)
	}
	if recipes, err := authoringContext.Recipes(t.Context(), workspace); err != nil {
		t.Fatalf("Recipes = (%+v, %v)", recipes, err)
	}
	if catalog, err := hooks.Catalog(t.Context(), workspace); err != nil {
		t.Fatalf("Hooks = (%+v, %v)", catalog, err)
	}
	if err := feedbackService.Record(t.Context(), agent.FeedbackSignal{
		SessionID: sessionID, Rating: protocol.FeedbackPositive, Text: "embedded integration",
	}); err != nil {
		t.Fatalf("Create feedback: %v", err)
	}
}

func requireContextManagement(t *testing.T, runtime *Connection, workspace string) {
	t.Helper()
	agentMemory := runtime.AgentMemory()
	knowledgeService := runtime.Knowledge()
	if agentMemory == nil || knowledgeService == nil {
		t.Fatal("context adapters were not advertised")
	}
	userTarget, err := agent.NewMemoryTarget(agent.MemoryUser, "")
	if err != nil {
		t.Fatal(err)
	}
	added, err := agentMemory.Add(t.Context(), userTarget, "integration preference")
	if err != nil {
		t.Fatalf("Add agent memory: %v", err)
	}
	items, err := agentMemory.Items(t.Context(), userTarget)
	if err != nil || len(items) != 1 || items[0].ID != added.ID {
		t.Fatalf("Items agent memory = (%+v, %v)", items, err)
	}
	pinned := true
	updated, err := agentMemory.Update(t.Context(), agent.MemoryPatch{ID: added.ID, Pinned: &pinned})
	if err != nil || !updated.Pinned {
		t.Fatalf("Update agent memory = (%+v, %v)", updated, err)
	}
	if deleteErr := agentMemory.Delete(t.Context(), added.ID); deleteErr != nil {
		t.Fatalf("Delete agent memory: %v", deleteErr)
	}
	entries, err := knowledgeService.Entries(t.Context(), workspace)
	if err != nil {
		t.Fatalf("Entries knowledge = (%+v, %v)", entries, err)
	}
	target, err := workspaceapi.NewKnowledgeTarget(workspaceapi.KnowledgeWorkingDirectory, workspace)
	if err != nil {
		t.Fatal(err)
	}
	before, err := knowledgeService.Document(t.Context(), target)
	if err != nil {
		t.Fatalf("read knowledge before save: %v", err)
	}
	update, err := before.Revise(target, "# Integration knowledge\n")
	if err != nil {
		t.Fatal(err)
	}
	if _, saveErr := knowledgeService.Save(t.Context(), update); saveErr != nil {
		t.Fatalf("Save knowledge: %v", saveErr)
	}
	document, err := knowledgeService.Document(t.Context(), target)
	if err != nil || document.Content != "# Integration knowledge\n" {
		t.Fatalf("Document knowledge = (%+v, %v)", document, err)
	}
}

func requireSessionPortability(t *testing.T, runtime *Connection, sessionID string) {
	t.Helper()
	markdown, err := runtime.ExportSession(t.Context(), session.ExportRequest{
		SessionID: sessionID, Format: session.MarkdownFormat,
	})
	if err != nil || len(markdown.Bytes()) == 0 || markdown.Importable() {
		t.Fatalf("Markdown ExportSession = (%q, %v)", markdown.Bytes(), err)
	}
	artifact, err := runtime.ExportSession(t.Context(), session.ExportRequest{
		SessionID: sessionID, Format: session.JSONFormat,
	})
	if err != nil || !artifact.Importable() {
		t.Fatalf("JSON ExportSession = (%q, %v)", artifact.Bytes(), err)
	}
	imported, err := runtime.ImportSession(t.Context(), session.ImportRequest{Artifact: artifact})
	if err != nil || imported.ID != sessionID {
		t.Fatalf("ImportSession = (%+v, %v)", imported, err)
	}
	rolledBack, err := runtime.RollbackSession(t.Context(), agent.RollbackSession{
		SessionID: sessionID, Scope: agent.RestoreHistory,
	})
	if err != nil || rolledBack.Session.ID != sessionID || len(rolledBack.Dropped) != 0 {
		t.Fatalf("RollbackSession = (%+v, %v)", rolledBack, err)
	}
}

func requireWorkspaceInspection(t *testing.T, runtime *Connection, path string) {
	t.Helper()
	if !runtime.Supports(changefeed.FilesChanged) {
		t.Fatal("embedded runtime did not advertise files.changed")
	}
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := runtime.Resolve(t.Context(), workspaceapi.ResolveRequest{Path: path})
	if err != nil || resolved.Path != canonical || !resolved.IsAvailable() {
		t.Fatalf("Resolve = (%+v, %v)", resolved, err)
	}
	path = resolved.Path
	known, err := runtime.List(t.Context())
	if err != nil || len(known) == 0 {
		t.Fatalf("List = (%+v, %v)", known, err)
	}
	files, err := runtime.Files(t.Context(), workspaceapi.FilesRequest{Workspace: path})
	if err != nil || len(files.Entries) != 2 || files.Entries[0].Path != "empty" ||
		files.Entries[0].Type != workspaceapi.FileEntryDirectory || files.Entries[1].Path != "main.go" {
		t.Fatalf("Files = (%+v, %v)", files, err)
	}
	headLimit, err := workspaceapi.NewHeadLineLimit(2)
	if err != nil {
		t.Fatal(err)
	}
	head, err := runtime.Head(t.Context(), workspaceapi.HeadRequest{Workspace: path, Path: "main.go", LineLimit: headLimit})
	if err != nil || len(head.Lines) != 2 || head.Lines[0].Text != "package main" {
		t.Fatalf("Head = (%+v, %v)", head, err)
	}
	searchLimit, err := workspaceapi.NewSearchResultLimit(20)
	if err != nil {
		t.Fatal(err)
	}
	found, err := runtime.Search(t.Context(), workspaceapi.SearchRequest{Workspace: path, Query: "answer", Limit: searchLimit})
	if err != nil || found.Total != 1 || len(found.Matches) != 1 {
		t.Fatalf("Search = (%+v, %v)", found, err)
	}
	content, err := runtime.Read(t.Context(), workspaceapi.ReadRequest{
		Workspace: path, Path: "main.go", Range: workspaceapi.WholeFileReadRange(),
		ByteLimit: workspaceapi.DefaultReadByteLimit(),
	})
	if err != nil || content.TotalLines != 4 || content.Content == "" {
		t.Fatalf("Read = (%+v, %v)", content, err)
	}
}

func configureIntegrationRuntime(t *testing.T) {
	t.Helper()
	t.Setenv("FLAME_PROVIDER", "anthropic")
	t.Setenv("ANTHROPIC_API_KEY", "test-key")
	t.Setenv("DEEPSEEK_API_KEY", "integration-env-key")
	t.Setenv("FLAME_MCP_SERVERS", "")
	t.Setenv("FLAME_A2A_AGENTS", "")
	t.Setenv("FLAME_A2A_RPC_ORIGINS", "")
}

func requireProviderMutationLifecycle(t *testing.T, runtime *Connection) {
	t.Helper()
	setBaseURL := models.ValueChange{Kind: models.SetValue, Value: "https://provider.integration.test"}
	setAPIKey := models.ValueChange{Kind: models.SetValue, Value: "integration-stored-key"}
	configured, err := runtime.UpdateProvider(t.Context(), models.UpdateProvider{
		Provider: "deepseek", BaseURL: &setBaseURL, APIKey: &setAPIKey,
	})
	if err != nil {
		t.Fatalf("configure provider: %v", err)
	}
	configuredBaseURL, hasConfiguredBaseURL := configured.BaseURL()
	configuredCredential, hasConfiguredCredential := configured.Credential()
	if !hasConfiguredBaseURL || configuredBaseURL != setBaseURL.Value || !hasConfiguredCredential || !configuredCredential.Stored() {
		t.Fatalf("configured provider = %+v", configured)
	}

	clear := models.ValueChange{Kind: models.ClearValue}
	fallback, err := runtime.UpdateProvider(t.Context(), models.UpdateProvider{
		Provider: "deepseek", BaseURL: &clear, APIKey: &clear,
	})
	if err != nil {
		t.Fatalf("clear provider: %v", err)
	}
	_, hasFallbackBaseURL := fallback.BaseURL()
	fallbackCredential, hasFallbackCredential := fallback.Credential()
	if hasFallbackBaseURL || !hasFallbackCredential || !fallbackCredential.FromEnvironment() {
		t.Fatalf("provider environment fallback = %+v", fallback)
	}
}

func openIntegrationRuntime(t *testing.T, workspace string) *Connection {
	t.Helper()
	runtime, err := Open(t.Context(), Config{
		DataDirectory: t.TempDir(), DefaultWorkspacePath: workspace,
		UserHomePath: t.TempDir(), ConfigDirectories: []string{t.TempDir()}, ClientVersion: "test",
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close() })
	return runtime
}

func requireSessionCatalog(t *testing.T, runtime *Connection, workspace string) agent.Session {
	t.Helper()
	created, err := runtime.CreateSession(t.Context(), agent.CreateSession{Title: "adapter session", Workspace: workspace})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	page, err := runtime.ListSessions(t.Context(), agent.SessionQuery{
		PageSize: catalogPageSize(t, 10), Search: "ADAPTER", Workspace: created.Workspace.Path,
	})
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].ID != created.ID {
		t.Fatalf("filtered sessions = %+v, want %s", page.Items, created.ID)
	}

	snapshot, err := runtime.GetSession(t.Context(), created.ID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if validateErr := snapshot.Validate(); validateErr != nil {
		t.Fatalf("snapshot: %v", validateErr)
	}
	if snapshot.Session.ID != created.ID || len(snapshot.Runs) != 0 || len(snapshot.Transcript) != 0 {
		t.Fatalf("snapshot = %+v", snapshot)
	}
	runs, err := runtime.ListRuns(t.Context(), agent.RunQuery{
		SessionID: created.ID, IncludeDescendants: true, PageSize: agent.DefaultPageSize(),
	})
	if err != nil || len(runs.Items) != 0 {
		t.Fatalf("ListRuns = (%+v, %v)", runs, err)
	}
	if _, err := runtime.GetRun(t.Context(), "run_missing"); !errors.Is(err, agent.ErrRunNotFound) {
		t.Fatalf("GetRun missing = %v, want ErrRunNotFound", err)
	}
	return created
}

func requireSessionMutation(t *testing.T, runtime *Connection, created agent.Session, workspace string) agent.Session {
	t.Helper()
	title, favorite := "renamed adapter session", true
	model := agent.ModelRef{Provider: created.Provider, Model: "integration-model"}
	updated, err := runtime.UpdateSession(t.Context(), agent.UpdateSession{
		SessionID: created.ID, Title: &title, Workspace: &workspace, Model: &model,
		Favorite: &favorite, ExpectedRevision: created.Revision,
	})
	if err != nil {
		t.Fatalf("UpdateSession: %v", err)
	}
	canonicalWorkspace, canonicalErr := filepath.EvalSymlinks(workspace)
	if canonicalErr != nil {
		t.Fatal(canonicalErr)
	}
	if updated.Title != title || updated.Workspace.Path != canonicalWorkspace || updated.Provider != model.Provider || updated.Model != model.Model ||
		!updated.Favorite || updated.Revision <= created.Revision {
		t.Fatalf("updated = %+v", updated)
	}
	forked, err := runtime.ForkSession(t.Context(), agent.ForkSession{SessionID: created.ID, Title: "forked adapter session"})
	if err != nil {
		t.Fatalf("ForkSession: %v", err)
	}
	if forked.ID == created.ID || forked.Title != "forked adapter session" {
		t.Fatalf("forked = %+v", forked)
	}
	return forked
}

func requireRuntimeCatalogs(t *testing.T, runtime *Connection, sessionID, workspace string) {
	t.Helper()
	models, err := runtime.ListModels(t.Context())
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models) == 0 {
		t.Fatal("ListModels returned no provider-qualified models")
	}
	providers, err := runtime.Providers(t.Context())
	if err != nil || len(providers) == 0 {
		t.Fatalf("Providers = (%+v, %v)", providers, err)
	}
	roles, err := runtime.Roles(t.Context())
	if err != nil {
		t.Fatalf("Roles = (%+v, %v)", roles, err)
	}
	sessionUsage, err := runtime.SessionUsage(t.Context(), sessionID)
	if err != nil || sessionUsage.SessionID != sessionID {
		t.Fatalf("SessionUsage = (%+v, %v)", sessionUsage, err)
	}
	usagePeriod, err := agent.RecentUsageDays(30)
	if err != nil {
		t.Fatal(err)
	}
	usageSummary, err := runtime.Summary(t.Context(), usagePeriod)
	if err != nil {
		t.Fatalf("Summary = (%+v, %v)", usageSummary, err)
	}
	days, recent, periodErr := usageSummary.Period.Days()
	if periodErr != nil || !recent || days != 30 {
		t.Fatalf("Summary period = (%d, %t, %v)", days, recent, periodErr)
	}
	if current, exists, getGoalErr := runtime.GetGoal(t.Context(), sessionID); getGoalErr != nil || exists {
		t.Fatalf("GetGoal without a goal = (%+v, %t, %v)", current, exists, getGoalErr)
	}
	if discovered, discoverErr := runtime.Discover(t.Context(), workspace); discoverErr != nil {
		t.Fatalf("Discover skills = (%+v, %v)", discovered, discoverErr)
	}
	if managed, managedErr := runtime.Managed(t.Context()); managedErr != nil {
		t.Fatalf("Managed skills = (%+v, %v)", managed, managedErr)
	}
	if proposals, proposalsErr := runtime.Proposals(t.Context(), workspace); proposalsErr != nil {
		t.Fatalf("Skill proposals = (%+v, %v)", proposals, proposalsErr)
	}
	if servers, serversErr := runtime.Servers(t.Context()); serversErr != nil {
		t.Fatalf("MCP servers = (%+v, %v)", servers, serversErr)
	}
	if tools, toolsErr := runtime.Tools(t.Context(), ""); toolsErr != nil {
		t.Fatalf("MCP tools = (%+v, %v)", tools, toolsErr)
	}
	requireMCPMutationLifecycle(t, runtime)
	requireScheduleLifecycle(t, runtime, workspace)
	if rules, listApprovalRulesErr := runtime.ListApprovalRules(t.Context(), sessionID); listApprovalRulesErr != nil || len(rules) != 0 {
		t.Fatalf("ListApprovalRules = (%+v, %v)", rules, listApprovalRulesErr)
	}

	applied, err := runtime.SetApprovalMode(t.Context(), protocol.ApprovalModeSafe)
	if err != nil || applied != protocol.ApprovalModeSafe {
		t.Fatalf("SetApprovalMode = (%q, %v)", applied, err)
	}
	mode, err := runtime.GetApprovalMode(t.Context())
	if err != nil || mode != protocol.ApprovalModeSafe {
		t.Fatalf("GetApprovalMode = (%q, %v)", mode, err)
	}
}

func requireMCPMutationLifecycle(t *testing.T, runtime *Connection) {
	t.Helper()
	timeout, err := mcp.NewHandshakeTimeout(5)
	if err != nil {
		t.Fatalf("NewHandshakeTimeout: %v", err)
	}
	authorization := mcp.AuthorizationChange{Kind: mcp.Set, Value: "Bearer integration-secret"}
	headers := mcp.HeadersChange{Kind: mcp.Set, Value: map[string]string{"X-Key": "integration-secret"}}
	candidate := mcp.Candidate{
		Name: "integration-docs", Enabled: false, Description: "Integration MCP",
		Connection: mcp.ConnectionInput{
			Transport: mcp.StreamableHTTP, URL: "https://mcp.example/tools",
			Authorization: &authorization, Headers: &headers,
		},
		HandshakeTimeout: timeout, DisabledTools: []string{"write"}, AutoApproveTools: []string{"search"},
	}
	created, err := runtime.CreateServer(t.Context(), candidate)
	if err != nil {
		t.Fatalf("Create MCP server: %v", err)
	}
	if validateResultErr := candidate.ValidateResult(created); validateResultErr != nil {
		t.Fatalf("created MCP server: %v", validateResultErr)
	}
	clearAuthorization := mcp.AuthorizationChange{Kind: mcp.Clear}
	clearHeaders := mcp.HeadersChange{Kind: mcp.Clear}
	description := "Updated integration MCP"
	updatedTimeout, err := mcp.NewHandshakeTimeout(10)
	if err != nil {
		t.Fatalf("NewHandshakeTimeout: %v", err)
	}
	update := mcp.ServerUpdate{
		Server: candidate.Name, Description: &description, HandshakeTimeout: &updatedTimeout,
		Connection: &mcp.ConnectionInput{
			Transport: mcp.StreamableHTTP, URL: candidate.Connection.URL,
			Authorization: &clearAuthorization, Headers: &clearHeaders,
		},
	}
	updated, err := runtime.UpdateServer(t.Context(), update)
	if err != nil {
		t.Fatalf("Update MCP server: %v", err)
	}
	if err := update.ValidateResult(updated); err != nil {
		t.Fatalf("updated MCP server: %v", err)
	}
	if err := runtime.DeleteServer(t.Context(), candidate.Name); err != nil {
		t.Fatalf("Delete MCP server: %v", err)
	}
}

func requireScheduleLifecycle(t *testing.T, runtime *Connection, workspace string) {
	t.Helper()
	created, err := runtime.Create(t.Context(), protocol.CreateScheduleRequest{
		Title: "Adapter schedule", Instructions: "review the workspace",
		Workspace: &protocol.WorkspaceRef{Path: workspace}, Cron: "0 9 * * 1-5",
	})
	if err != nil {
		t.Fatalf("Create schedule: %v", err)
	}
	listed, err := runtime.Schedules(t.Context())
	if err != nil || len(listed) != 1 || listed[0].ID != created.ID {
		t.Fatalf("Schedules = (%+v, %v)", listed, err)
	}
	enabled := false
	updated, err := runtime.Update(t.Context(), protocol.UpdateScheduleRequest{
		ID: created.ID, ExpectedRevision: created.Revision, Enabled: &enabled,
	})
	if err != nil || updated.Enabled || updated.Revision <= created.Revision {
		t.Fatalf("Update schedule = (%+v, %v)", updated, err)
	}
	if err := runtime.Delete(t.Context(), created.ID); err != nil {
		t.Fatalf("Delete schedule: %v", err)
	}
}

func requireSessionDeletion(t *testing.T, runtime *Connection, sessionIDs ...string) {
	t.Helper()
	for _, sessionID := range sessionIDs {
		if err := runtime.DeleteSession(t.Context(), agent.DeleteSession{SessionID: sessionID}); err != nil {
			t.Fatalf("DeleteSession %s: %v", sessionID, err)
		}
	}
	_, err := runtime.GetSession(t.Context(), sessionIDs[0])
	if !errors.Is(err, agent.ErrSessionNotFound) {
		t.Fatalf("GetSession after delete = %v, want ErrSessionNotFound", err)
	}
	problem, ok := errors.AsType[protocol.ProblemError](err)
	if !ok || problem.Problem().Type != protocol.ErrSessionNotFound.Error() {
		t.Fatalf("structured GetSession error = %T %v", err, err)
	}
}

func requireClosedRuntime(t *testing.T, runtime *Connection) {
	t.Helper()
	if err := runtime.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := runtime.ListModels(t.Context()); !errors.Is(err, agent.ErrDisconnected) {
		t.Fatalf("ListModels after Close = %v, want ErrDisconnected", err)
	}
}

func TestOwnerOpensOnceAndRefusesReopenAfterClose(t *testing.T) {
	configureIntegrationRuntime(t)

	owner := NewOwner(Config{
		DataDirectory: t.TempDir(), DefaultWorkspacePath: t.TempDir(),
		UserHomePath: t.TempDir(), ConfigDirectories: []string{t.TempDir()}, ClientVersion: "test",
	})
	first, err := owner.Connection(t.Context())
	if err != nil {
		t.Fatalf("first Runtime: %v", err)
	}
	second, err := owner.Connection(t.Context())
	if err != nil {
		t.Fatalf("second Runtime: %v", err)
	}
	if first != second {
		t.Fatal("owner opened more than one connection")
	}
	firstProfile := first.Profile()
	secondProfile := second.Profile()
	firstProfile.RuntimeTopics[0] = "mutated"
	if secondProfile.RuntimeTopics[0] == "mutated" {
		t.Fatal("owner leaked mutable profile state")
	}
	if first.AgentMemory() == nil || first.Knowledge() == nil ||
		first.DiagnosticTools() == nil || first.AuthoringContext() == nil ||
		first.Hooks() == nil || first.Feedback() == nil {
		t.Fatal("connection adapters were not composed")
	}
	if err := owner.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := owner.Connection(t.Context()); !errors.Is(err, agent.ErrDisconnected) {
		t.Fatalf("Connection after Close = %v, want ErrDisconnected", err)
	}
}
