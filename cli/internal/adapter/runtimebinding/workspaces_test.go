package runtimebinding

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	flameruntime "github.com/Tangerg/flame/runtime"
	"github.com/Tangerg/flame/runtime/protocol"

	"github.com/Tangerg/flame/cli/internal/domain/agent"
	"github.com/Tangerg/flame/cli/internal/domain/workspace"
)

type workspaceBindingStub struct {
	resolved     *protocol.WorkspaceInfo
	known        *protocol.Page[protocol.WorkspaceSummary]
	changes      *protocol.Page[protocol.WorkspaceFileChange]
	changesErr   error
	changesCalls int
	diff         *protocol.Diff
	diffCalls    int
	head         *protocol.FileHead
	search       *protocol.GrepResult
	files        *protocol.Page[protocol.FileEntry]
	filePages    map[string]*protocol.Page[protocol.FileEntry]
	fileCalls    []protocol.ListFilesRequest
	content      *protocol.FileContent
}

func (w *workspaceBindingStub) ResolveWorkspace(context.Context, protocol.ResolveWorkspaceRequest, flameruntime.CallOptions) (*protocol.WorkspaceInfo, error) {
	return w.resolved, nil
}

func (w *workspaceBindingStub) ListWorkspaces(context.Context, flameruntime.CallOptions) (*protocol.Page[protocol.WorkspaceSummary], error) {
	return w.known, nil
}

func (w *workspaceBindingStub) ListWorkspaceFileChanges(context.Context, protocol.WorkspaceQuery, flameruntime.CallOptions) (*protocol.Page[protocol.WorkspaceFileChange], error) {
	w.changesCalls++
	return w.changes, w.changesErr
}

func TestWorkspaceAdapterProjectsVersionControlUnavailability(t *testing.T) {
	t.Parallel()
	stub := &workspaceBindingStub{changesErr: protocol.ErrVcsUnavailable}
	runtime := &Connection{
		workspaces: stub, meta: requestMeta("test"),
		profile: Profile{Features: map[string]Feature{
			protocol.FeatureGit: {Enabled: true},
		}},
	}

	_, err := runtime.Changes(t.Context(), "/workspace")
	if !errors.Is(err, workspace.ErrVersionControlUnavailable) ||
		!errors.Is(err, protocol.ErrVcsUnavailable) {
		t.Fatalf("Changes error = %v, want workspace and protocol identities", err)
	}
}

func (w *workspaceBindingStub) GetWorkspaceDiff(context.Context, protocol.GetDiffRequest, flameruntime.CallOptions) (*protocol.Diff, error) {
	w.diffCalls++
	return w.diff, nil
}

func (w *workspaceBindingStub) GetWorkspaceFileHead(context.Context, protocol.GetFileHeadRequest, flameruntime.CallOptions) (*protocol.FileHead, error) {
	return w.head, nil
}

func (w *workspaceBindingStub) SearchWorkspaceFiles(context.Context, protocol.GrepRequest, flameruntime.CallOptions) (*protocol.GrepResult, error) {
	return w.search, nil
}

func (w *workspaceBindingStub) ListWorkspaceFiles(_ context.Context, request protocol.ListFilesRequest, _ flameruntime.CallOptions) (*protocol.Page[protocol.FileEntry], error) {
	w.fileCalls = append(w.fileCalls, request)
	if w.filePages != nil {
		return w.filePages[request.Cursor], nil
	}
	return w.files, nil
}

func (w *workspaceBindingStub) ReadWorkspaceFile(context.Context, protocol.ReadFileRequest, flameruntime.CallOptions) (*protocol.FileContent, error) {
	return w.content, nil
}

func TestWorkspaceAdapterProjectsEveryReadShape(t *testing.T) {
	t.Parallel()
	added, removed, size := 4, 1, int64(120)
	lastActive := time.Date(2026, time.August, 12, 9, 0, 0, 0, time.UTC)
	stub := &workspaceBindingStub{
		resolved: &protocol.WorkspaceInfo{Ref: protocol.WorkspaceRef{Path: "/workspace"}, ProjectRoot: "/workspace", Availability: protocol.WorkspaceAvailable},
		known: protocol.NewPage([]protocol.WorkspaceSummary{{
			Workspace: protocol.WorkspaceInfo{Ref: protocol.WorkspaceRef{Path: "/workspace"}, ProjectRoot: "/workspace", Availability: protocol.WorkspaceAvailable},
			Name:      "workspace", SessionCount: 2, LastActiveAt: &lastActive,
		}}),
		changes: protocol.NewPage([]protocol.WorkspaceFileChange{{Path: "main.go", Status: protocol.FileStatusModified, Added: &added, Removed: &removed}}),
		diff: &protocol.Diff{Files: []protocol.FileDiff{{
			Path: "main.go", Status: protocol.FileStatusModified, Added: &added, Removed: &removed,
			Rows: []protocol.DiffRow{{Type: protocol.DiffRowAdded, RightLine: 1, Code: "package main"}},
		}}},
		head:   &protocol.FileHead{Lines: []protocol.FileLine{{LineNumber: 1, Text: "package main"}}},
		search: &protocol.GrepResult{Matches: []protocol.GrepMatch{{Path: "main.go", LineNumber: 1, Text: "package main"}}, Total: 1},
		filePages: map[string]*protocol.Page[protocol.FileEntry]{
			"": protocol.NewPageWithCursor([]protocol.FileEntry{{Path: "internal", Name: "internal", Type: protocol.FileEntryDir}}, "next"),
			"next": protocol.NewPage([]protocol.FileEntry{{
				Path: "main.go", Name: "main.go", Type: protocol.FileEntryFile, SizeBytes: &size,
				ModifiedAt: time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC),
			}}),
		},
		content: &protocol.FileContent{Content: "package main\n", TotalLines: 1},
	}
	runtime := &Connection{
		workspaces: stub, meta: requestMeta("test"),
		profile: Profile{Features: map[string]Feature{
			protocol.FeatureGit: {Enabled: true},
		}},
	}

	resolved, err := runtime.Resolve(t.Context(), workspace.ResolveRequest{Path: "/workspace"})
	if err != nil || resolved.Path != "/workspace" || !resolved.IsAvailable() {
		t.Fatalf("Resolve = (%+v, %v)", resolved, err)
	}
	known, err := runtime.List(t.Context())
	if err != nil || len(known) != 1 || known[0].Sessions != 2 || known[0].LastActive == nil || !known[0].LastActive.Equal(lastActive) {
		t.Fatalf("List = (%+v, %v)", known, err)
	}
	lastActive = lastActive.Add(time.Hour)
	if known[0].LastActive.Equal(lastActive) {
		t.Fatal("workspace projection aliases runtime last-active storage")
	}
	changes, err := runtime.Changes(t.Context(), "/workspace")
	if err != nil || len(changes) != 1 || changes[0].Stat() != "+4 -1" {
		t.Fatalf("Changes = (%+v, %v)", changes, err)
	}
	diff, err := runtime.Diff(t.Context(), workspace.DiffRequest{
		Workspace: "/workspace", Format: protocol.DiffFormatRows, RowLimit: workspace.DefaultDiffRowLimit(),
	})
	if err != nil || diff.Text() != "diff -- main.go (modified)\n+package main" {
		t.Fatalf("Diff = (%+v, %v)", diff, err)
	}
	stub.diff.Files[0].Rows[0].Code = "mutated"
	if diff.Text() != "diff -- main.go (modified)\n+package main" {
		t.Fatal("workspace diff projection aliases runtime row storage")
	}
	head, err := runtime.Head(t.Context(), workspace.HeadRequest{
		Workspace: "/workspace", Path: "main.go", LineLimit: workspace.DefaultHeadLineLimit(),
	})
	if err != nil || len(head.Lines) != 1 {
		t.Fatalf("Head = (%+v, %v)", head, err)
	}
	stub.head.Lines[0].Text = "mutated"
	if head.Lines[0].Text != "package main" {
		t.Fatal("file head projection aliases runtime line storage")
	}
	search, err := runtime.Search(t.Context(), workspace.SearchRequest{
		Workspace: "/workspace", Query: "main", Limit: workspace.DefaultSearchResultLimit(),
	})
	if err != nil || search.Total != 1 || len(search.Matches) != 1 {
		t.Fatalf("Search = (%+v, %v)", search, err)
	}
	stub.search.Matches[0].Text = "mutated"
	if search.Matches[0].Text != "package main" {
		t.Fatal("workspace search projection aliases runtime match storage")
	}
	files, err := runtime.Files(t.Context(), workspace.FilesRequest{Workspace: "/workspace"})
	if err != nil || len(files.Entries) != 2 || files.Entries[0].Type != protocol.FileEntryDir ||
		files.Entries[1].Type != protocol.FileEntryFile || *files.Entries[1].SizeBytes != size {
		t.Fatalf("Files = (%+v, %v)", files, err)
	}
	if len(stub.fileCalls) != 2 || stub.fileCalls[0].Cursor != "" ||
		stub.fileCalls[1].Cursor != "next" || stub.fileCalls[1].Limit == nil || *stub.fileCalls[1].Limit != workspaceFilePageLimit {
		t.Fatalf("ListWorkspaceFiles calls = %+v", stub.fileCalls)
	}
	content, err := runtime.Read(t.Context(), workspace.ReadRequest{
		Workspace: "/workspace", Path: "main.go", Range: workspace.WholeFileReadRange(),
		ByteLimit: workspace.DefaultReadByteLimit(),
	})
	if err != nil || content.Content != "package main\n" || content.Window() != "1 lines" {
		t.Fatalf("Read = (%+v, %v)", content, err)
	}

	added = 99
	if *changes[0].Added != 4 {
		t.Fatal("workspace projection retained a mutable protocol pointer")
	}
}

func TestWorkspaceAdapterRejectsRepeatedRuntimeIdentity(t *testing.T) {
	t.Parallel()
	summary := protocol.WorkspaceSummary{
		Workspace: protocol.WorkspaceInfo{
			Ref: protocol.WorkspaceRef{Path: "/workspace"}, ProjectRoot: "/workspace",
			Availability: protocol.WorkspaceAvailable,
		},
		Name: "workspace",
	}
	runtime := &Connection{
		workspaces: &workspaceBindingStub{known: protocol.NewPage([]protocol.WorkspaceSummary{summary, summary})},
		meta:       requestMeta("test"),
	}

	_, err := runtime.List(t.Context())
	if err == nil || !strings.Contains(err.Error(), `list workspaces repeats "/workspace"`) {
		t.Fatalf("List error = %v, want repeated workspace identity failure", err)
	}
}

func TestWorkspaceAdapterRejectsCatalogOrderViolations(t *testing.T) {
	t.Parallel()
	active := time.Date(2026, 9, 4, 8, 0, 0, 0, time.UTC)
	summary := func(path, name string, lastActive *time.Time) protocol.WorkspaceSummary {
		return protocol.WorkspaceSummary{
			Workspace: protocol.WorkspaceInfo{
				Ref: protocol.WorkspaceRef{Path: path}, ProjectRoot: path,
				Availability: protocol.WorkspaceAvailable,
			},
			Name: name, SessionCount: 1, LastActiveAt: lastActive,
		}
	}
	older := active.Add(-time.Hour)
	zero := time.Time{}
	for _, test := range []struct {
		name   string
		values []protocol.WorkspaceSummary
	}{
		{
			name:   "activity time is missing",
			values: []protocol.WorkspaceSummary{summary("/workspace", "workspace", nil)},
		},
		{
			name:   "activity time is zero",
			values: []protocol.WorkspaceSummary{summary("/workspace", "workspace", &zero)},
		},
		{
			name: "activity time ascends",
			values: []protocol.WorkspaceSummary{
				summary("/older", "older", &older),
				summary("/newer", "newer", &active),
			},
		},
		{
			name: "equal-time path descends",
			values: []protocol.WorkspaceSummary{
				summary("/zeta", "zeta", &active),
				summary("/alpha", "alpha", &active),
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			runtime := &Connection{
				workspaces: &workspaceBindingStub{known: protocol.NewPage(test.values)},
				meta:       requestMeta("test"),
			}
			_, err := runtime.List(t.Context())
			requireRuntimeContractViolation(t, err)
		})
	}
}

func TestWorkspaceAdapterRejectsRepeatedChangePath(t *testing.T) {
	t.Parallel()
	change := protocol.WorkspaceFileChange{Path: "main.go", Status: protocol.FileStatusModified}
	runtime := &Connection{
		workspaces: &workspaceBindingStub{changes: protocol.NewPage([]protocol.WorkspaceFileChange{change, change})},
		meta:       requestMeta("test"),
		profile: Profile{Features: map[string]Feature{
			protocol.FeatureGit: {Enabled: true},
		}},
	}

	_, err := runtime.Changes(t.Context(), "/workspace")
	if err == nil || !strings.Contains(err.Error(), `list workspace changes repeats "main.go"`) {
		t.Fatalf("Changes error = %v, want repeated path failure", err)
	}
}

func TestWorkspaceAdapterRejectsGitReadsBeforeCallingBinding(t *testing.T) {
	t.Parallel()
	stub := &workspaceBindingStub{}
	runtime := &Connection{workspaces: stub}
	if _, err := runtime.Changes(t.Context(), "/workspace"); err == nil || !errors.Is(err, agent.ErrIncompatibleRuntime) {
		t.Fatalf("Changes error = %v, want ErrIncompatibleRuntime", err)
	}
	if _, err := runtime.Diff(t.Context(), workspace.DiffRequest{
		Workspace: "/workspace", RowLimit: workspace.DefaultDiffRowLimit(),
	}); err == nil || !errors.Is(err, agent.ErrIncompatibleRuntime) {
		t.Fatalf("Diff error = %v, want ErrIncompatibleRuntime", err)
	}
	if stub.changesCalls != 0 || stub.diffCalls != 0 {
		t.Fatalf("git reads reached binding without capability: changes=%d diff=%d", stub.changesCalls, stub.diffCalls)
	}
}

func TestWorkspaceFilesRejectsCyclicRuntimePagination(t *testing.T) {
	t.Parallel()
	stub := &workspaceBindingStub{filePages: map[string]*protocol.Page[protocol.FileEntry]{
		"":      protocol.NewPageWithCursor([]protocol.FileEntry{}, "next"),
		"next":  protocol.NewPageWithCursor([]protocol.FileEntry{}, "later"),
		"later": protocol.NewPageWithCursor([]protocol.FileEntry{}, "next"),
	}}
	runtime := &Connection{workspaces: stub, meta: requestMeta("test")}

	_, err := runtime.Files(t.Context(), workspace.FilesRequest{Workspace: "/workspace"})
	if err == nil || !strings.Contains(err.Error(), "cyclic continuation cursor") {
		t.Fatalf("Files error = %v, want cyclic continuation cursor", err)
	}
}

func TestWorkspaceFilesRejectsPagesOutsideRuntimeOrder(t *testing.T) {
	t.Parallel()
	size := int64(1)
	file := func(path string) protocol.FileEntry {
		return protocol.FileEntry{Path: path, Name: path, Type: protocol.FileEntryFile, SizeBytes: &size}
	}
	directory := func(path string) protocol.FileEntry {
		return protocol.FileEntry{Path: path, Name: path, Type: protocol.FileEntryDir}
	}
	for _, test := range []struct {
		name  string
		pages map[string]*protocol.Page[protocol.FileEntry]
	}{
		{
			name: "path descends within a page",
			pages: map[string]*protocol.Page[protocol.FileEntry]{
				"": protocol.NewPage([]protocol.FileEntry{file("b.go"), file("a.go")}),
			},
		},
		{
			name: "directory follows a file across pages",
			pages: map[string]*protocol.Page[protocol.FileEntry]{
				"":     protocol.NewPageWithCursor([]protocol.FileEntry{file("a.go")}, "next"),
				"next": protocol.NewPage([]protocol.FileEntry{directory("docs")}),
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			runtime := &Connection{
				workspaces: &workspaceBindingStub{filePages: test.pages},
				meta:       requestMeta("test"),
			}
			_, err := runtime.Files(t.Context(), workspace.FilesRequest{Workspace: "/workspace"})
			requireRuntimeContractViolation(t, err)
		})
	}
}

func TestWorkspaceFilesValidateCompleteRuntimeEntriesBeforeProjection(t *testing.T) {
	t.Parallel()

	runtime := &Connection{
		workspaces: &workspaceBindingStub{files: protocol.NewPage([]protocol.FileEntry{{
			Path: "main.go", Type: protocol.FileEntryFile,
		}})},
		meta: requestMeta("test"),
	}

	_, err := runtime.Files(t.Context(), workspace.FilesRequest{Workspace: "/workspace"})
	if err == nil || !strings.Contains(err.Error(), "name") {
		t.Fatalf("Files error = %v, want missing runtime name failure", err)
	}
	requireRuntimeContractViolation(t, err)
}

func TestWorkspaceUnpageableListsRejectContinuation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		stub *workspaceBindingStub
		call func(context.Context, *Connection) error
	}{
		{
			name: "catalog",
			stub: &workspaceBindingStub{known: protocol.NewPageWithCursor([]protocol.WorkspaceSummary{}, "next")},
			call: func(ctx context.Context, runtime *Connection) error {
				_, err := runtime.List(ctx)
				return err
			},
		},
		{
			name: "changes",
			stub: &workspaceBindingStub{changes: protocol.NewPageWithCursor([]protocol.WorkspaceFileChange{}, "next")},
			call: func(ctx context.Context, runtime *Connection) error {
				_, err := runtime.Changes(ctx, "/workspace")
				return err
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			runtime := &Connection{workspaces: test.stub, meta: requestMeta("test")}
			if test.name == "changes" {
				runtime.profile.Features = map[string]Feature{
					protocol.FeatureGit: {Enabled: true},
				}
			}
			err := test.call(t.Context(), runtime)
			if err == nil || !strings.Contains(err.Error(), "continuation cursor") {
				t.Fatalf("list error = %v, want continuation cursor failure", err)
			}
		})
	}
}

func TestWorkspaceAdapterRejectsNilResponses(t *testing.T) {
	t.Parallel()
	runtime := &Connection{workspaces: &workspaceBindingStub{}, meta: requestMeta("test")}
	if _, err := runtime.Read(t.Context(), workspace.ReadRequest{
		Workspace: "/workspace", Path: "main.go", Range: workspace.WholeFileReadRange(),
		ByteLimit: workspace.DefaultReadByteLimit(),
	}); err == nil {
		t.Fatal("nil file content was accepted")
	}
}
