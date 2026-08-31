package sqlite

import (
	"fmt"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
)

const sessionColumns = `id, title, title_search, workspace_path, workspace_search, parent_id, started_at, updated_at, provider, model, reasoning_effort, favorite, isolated, revision`

// rowToSession decodes one DB row into a product session.Session. Execution
// continuation state deliberately lives in its dedicated sidecar table, never
// in this session projection.
func rowToSession(scanner interface {
	Scan(dest ...any) error
}) (session.Session, error) {
	var (
		snapshot        session.Snapshot
		startedAtNanos  int64
		updatedAtNanos  int64
		favoriteInt     int64
		isolatedInt     int64
		provider        string
		model           string
		reasoningEffort string
		workspacePath   string
		titleSearch     string
		workspaceSearch string
	)
	if err := scanner.Scan(
		&snapshot.ID, &snapshot.Title, &titleSearch, &workspacePath, &workspaceSearch, &snapshot.ParentID,
		&startedAtNanos, &updatedAtNanos, &provider, &model, &reasoningEffort,
		&favoriteInt, &isolatedInt, &snapshot.Revision,
	); err != nil {
		return session.Session{}, err
	}
	expectedTitleSearch, err := session.NormalizeCatalogText(snapshot.Title)
	if err != nil || titleSearch != expectedTitleSearch {
		return session.Session{}, fmt.Errorf("sqlite: Session %q has invalid title search material", snapshot.ID)
	}
	expectedWorkspaceSearch, err := session.NormalizeCatalogText(workspacePath)
	if err != nil || workspaceSearch != expectedWorkspaceSearch {
		return session.Session{}, fmt.Errorf("sqlite: Session %q has invalid workspace search material", snapshot.ID)
	}
	snapshot.StartedAt = time.Unix(0, startedAtNanos).UTC()
	snapshot.UpdatedAt = time.Unix(0, updatedAtNanos).UTC()
	snapshot.Favorite = favoriteInt != 0
	snapshot.Isolated = isolatedInt != 0
	workspace, err := session.NewWorkspace(workspacePath)
	if err != nil {
		return session.Session{}, err
	}
	snapshot.Workspace = workspace
	selection, err := modelref.NewWithReasoningEffort(provider, model, reasoningEffort)
	if err != nil {
		return session.Session{}, err
	}
	snapshot.Selection = selection
	value, err := session.Restore(snapshot)
	if err != nil {
		return session.Session{}, err
	}
	return value, nil
}
