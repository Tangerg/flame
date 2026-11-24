package runtime

import (
	"errors"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

// A Project is how a user finds the same work again: membership must survive a
// restart, and neither archiving nor leaving may remove or move anything else.
func TestProjectKeepsItsSessionsAcrossRestartWithoutCascading(t *testing.T) {
	t.Setenv("FLAME_PROVIDER", "anthropic")
	t.Setenv("FLAME_MODEL", "claude-opus-5")
	config := Config{
		DataDirectory:        testsupport.TempDirectory(t),
		DefaultWorkspacePath: testsupport.TempDirectory(t),
		UserHomePath:         testsupport.TempDirectory(t),
		ConfigDirectories:    []string{testsupport.TempDirectory(t)},
	}
	rt, err := Open(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	command := func(key string) protocol.CommandOptions { return protocol.CommandOptions{IdempotencyKey: key} }

	research, err := rt.CreateProject(t.Context(), protocol.CreateProjectRequest{Name: "Payments API"}, command("create-research"))
	if err != nil {
		_ = rt.Close()
		t.Fatal(err)
	}
	review, err := rt.CreateProject(t.Context(), protocol.CreateProjectRequest{Name: "Compliance review"}, command("create-review"))
	if err != nil {
		_ = rt.Close()
		t.Fatal(err)
	}
	member, err := rt.CreateSession(t.Context(), protocol.CreateSessionRequest{Title: "Compare vendors", ProjectID: research.ID}, command("create-member"))
	if err != nil {
		_ = rt.Close()
		t.Fatal(err)
	}
	moved, err := rt.CreateSession(t.Context(), protocol.CreateSessionRequest{Title: "Draft checklist"}, command("create-moved"))
	if err == nil {
		moved, err = rt.UpdateSession(t.Context(), protocol.UpdateSessionRequest{
			SessionID: moved.ID, ExpectedRevision: moved.Revision,
			Project: &protocol.SessionProjectChange{Type: protocol.SessionProjectSet, ProjectID: review.ID},
		}, command("join-review"))
	}
	if err != nil {
		_ = rt.Close()
		t.Fatal(err)
	}
	_, err = rt.CreateSession(t.Context(), protocol.CreateSessionRequest{ProjectID: "prj_missing"}, command("create-orphan"))
	if !errors.Is(err, protocol.ErrProjectNotFound) {
		_ = rt.Close()
		t.Fatalf("joining an unknown Project = %v, want project_not_found", err)
	}
	if _, err := rt.UpdateProject(t.Context(), protocol.UpdateProjectRequest{
		ProjectID: research.ID, ExpectedRevision: research.Revision, Archived: new(true),
	}, command("archive-research")); err != nil {
		_ = rt.Close()
		t.Fatal(err)
	}
	if err := rt.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	members := func(projectID string) []string {
		t.Helper()
		page, err := reopened.ListSessions(t.Context(), protocol.ListSessionsRequest{ProjectID: projectID}, protocol.CallOptions{})
		if err != nil {
			t.Fatal(err)
		}
		ids := make([]string, 0, len(page.Data))
		for _, value := range page.Data {
			ids = append(ids, value.ID)
		}
		return ids
	}
	if got := members(research.ID); len(got) != 1 || got[0] != member.ID {
		t.Fatalf("archived Project members after restart = %v, want [%s]", got, member.ID)
	}
	if got := members(review.ID); len(got) != 1 || got[0] != moved.ID {
		t.Fatalf("Project members after restart = %v, want [%s]", got, moved.ID)
	}
	archived, err := reopened.ListProjects(t.Context(), protocol.ListProjectsRequest{Archived: true}, protocol.CallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	active, err := reopened.ListProjects(t.Context(), protocol.ListProjectsRequest{}, protocol.CallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(archived.Data) != 1 || archived.Data[0].ID != research.ID || len(active.Data) != 1 || active.Data[0].ID != review.ID {
		t.Fatalf("catalogs after archive = archived %+v, active %+v", archived.Data, active.Data)
	}

	left, err := reopened.UpdateSession(t.Context(), protocol.UpdateSessionRequest{
		SessionID: moved.ID, ExpectedRevision: moved.Revision,
		Project: &protocol.SessionProjectChange{Type: protocol.SessionProjectClear},
	}, command("leave-review"))
	if err != nil {
		t.Fatal(err)
	}
	if left.ProjectID != "" || len(members(review.ID)) != 0 {
		t.Fatalf("leaving kept membership: session %+v, members %v", left, members(review.ID))
	}
	if _, err := reopened.GetSession(t.Context(), protocol.GetSessionRequest{SessionID: moved.ID}, protocol.CallOptions{}); err != nil {
		t.Fatalf("leaving a Project removed the Session: %v", err)
	}
	if _, err := reopened.GetProject(t.Context(), protocol.GetProjectRequest{ProjectID: review.ID}, protocol.CallOptions{}); err != nil {
		t.Fatalf("losing its last member removed the Project: %v", err)
	}
}
