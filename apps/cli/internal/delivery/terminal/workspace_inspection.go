package terminal

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Tangerg/flame/apps/cli/internal/domain/workspace"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

// workspaceScreens inspects the current workspace: known workspaces, changes,
// diffs, search, listings and files. changesView is the open changes read,
// updated in place as changes arrive.
type workspaceScreens struct {
	screenHost
	service     Workspaces
	changesView *readerQuery
}

func (w *workspaceScreens) ShowWorkspaces() {
	w.runWorkspaceQuery("loading workspaces",
		func(ctx context.Context) (readerDocument, error) {
			values, err := w.service.List(ctx)
			if err != nil {
				return readerDocument{}, err
			}
			lines := make([]string, 0, len(values))
			for _, value := range values {
				label := value.Name + "  " + value.Workspace.Path
				if value.Sessions > 0 {
					label += fmt.Sprintf("  · %d sessions", value.Sessions)
				}
				if !value.Workspace.IsAvailable() {
					label += "  · missing"
				}
				if value.Workspace.ProjectRoot != "" && value.Workspace.ProjectRoot != value.Workspace.Path {
					label += "  · project " + value.Workspace.ProjectRoot
				}
				if value.LastActive != nil {
					label += "  · active " + value.LastActive.Format(time.RFC3339)
				}
				lines = append(lines, label)
			}
			return paragraphDocument("Runtime workspaces", fmt.Sprintf("%d known", len(values)), lines), nil
		})
}

func (w *workspaceScreens) ShowWorkspaceChanges() {
	path := w.session.current.Workspace.Path
	w.changesView = &readerQuery{status: "loading workspace changes", read: func(ctx context.Context) (readerDocument, error) {
		changes, err := w.service.Changes(ctx, path)
		if err != nil {
			return readerDocument{}, err
		}
		return workspaceChangesDocument(path, changes), nil
	}}
	w.reader.show(w.changesView)
}

func (w *workspaceScreens) ShowWorkspaceDiff(argument string) error {
	selection, err := parseWorkspaceDiffSelection(argument)
	if err != nil {
		return err
	}
	request := workspace.DiffRequest{
		Workspace: w.session.current.Workspace.Path, Path: selection.path,
		Mode: selection.mode, Format: selection.format, RowLimit: selection.limit,
	}
	w.runWorkspaceQuery("loading workspace diff",
		func(ctx context.Context) (readerDocument, error) {
			diff, err := w.service.Diff(ctx, request)
			if err != nil {
				return readerDocument{}, err
			}
			return workspaceDiffDocument(request, diff), nil
		})
	return nil
}

func (w *workspaceScreens) PreviewWorkspaceFile(argument string) error {
	selection, err := parseWorkspacePreviewSelection(argument)
	if err != nil {
		return err
	}
	lineRange, err := workspace.NewReadLineRange(1, selection.lines)
	if err != nil {
		return err
	}
	request := workspace.ReadRequest{
		Workspace: w.session.current.Workspace.Path, Path: selection.path,
		Range: lineRange, ByteLimit: workspace.DefaultReadByteLimit(),
	}
	w.runWorkspaceQuery("loading file preview",
		func(ctx context.Context) (readerDocument, error) {
			content, err := w.service.Read(ctx, request)
			if err != nil {
				return readerDocument{}, err
			}
			detail := fmt.Sprintf("%s · %s", request.Path, content.Window())
			if content.Truncated {
				detail += " · truncated"
			}
			return codeDocument("File preview", detail, content.Content, request.Path, true), nil
		})
	return nil
}

func (w *workspaceScreens) SearchWorkspace(argument string) error {
	selection, err := parseWorkspaceSearchSelection(argument)
	if err != nil {
		return err
	}
	request := workspace.SearchRequest{
		Workspace: w.session.current.Workspace.Path, Query: selection.query, Path: selection.path, Limit: selection.limit,
		ScanAll: selection.scanAll,
	}
	w.runWorkspaceQuery("searching workspace",
		func(ctx context.Context) (readerDocument, error) {
			result, err := w.service.Search(ctx, request)
			if err != nil {
				return readerDocument{}, err
			}
			lines := make([]string, 0, len(result.Matches))
			for _, match := range result.Matches {
				lines = append(lines, fmt.Sprintf("%s:%d  %s", match.Path, match.LineNumber, match.Text))
			}
			detail := workspaceSearchSummary(result)
			if request.Path != "" {
				detail += " · " + request.Path
			}
			return paragraphDocument("Workspace search · "+request.Query, detail, lines), nil
		})
	return nil
}

func workspaceSearchSummary(result workspace.SearchResult) string {
	if result.Complete {
		return fmt.Sprintf("%d/%d matches", len(result.Matches), result.Total)
	}
	return fmt.Sprintf("%d shown · at least %d matches · %d files unread · %d files skipped",
		len(result.Matches), result.Total, result.Coverage.RemainingFiles, result.Coverage.SkippedFiles)
}

func (w *workspaceScreens) BrowseWorkspace(argument string) error {
	selection, err := parseWorkspaceFilesSelection(argument)
	if err != nil {
		return err
	}
	request := workspace.FilesRequest{
		Workspace: w.session.current.Workspace.Path, Path: selection.path, Glob: selection.glob,
		Recursive: selection.recursive, IncludeIgnored: selection.includeIgnored,
	}
	w.runWorkspaceQuery("browsing workspace",
		func(ctx context.Context) (readerDocument, error) {
			listing, err := w.service.Files(ctx, request)
			if err != nil {
				return readerDocument{}, err
			}
			lines := make([]string, 0, len(listing.Entries))
			for _, entry := range listing.Entries {
				kind := string(entry.Type)
				if entry.Type == protocol.FileEntryDir {
					kind = "dir"
				}
				line := fmt.Sprintf("%-7s %s", kind, entry.Path)
				var metadata []string
				if entry.SizeBytes != nil {
					metadata = append(metadata, fmt.Sprintf("%d B", *entry.SizeBytes))
				}
				if !entry.ModifiedAt.IsZero() {
					metadata = append(metadata, entry.ModifiedAt.Format(time.RFC3339Nano))
				}
				if len(metadata) > 0 {
					line += "  · " + strings.Join(metadata, " · ")
				}
				lines = append(lines, line)
			}
			details := []string{fmt.Sprintf("%d entries", len(listing.Entries))}
			if request.Glob != "" {
				details = append(details, "glob "+request.Glob)
			}
			if request.Recursive {
				details = append(details, "recursive")
			}
			if request.IncludeIgnored {
				details = append(details, "including ignored")
			}
			title := "Workspace files"
			if request.Path != "" {
				title += " · " + request.Path
			}
			return codeDocument(title, strings.Join(details, " · "), strings.Join(lines, "\n"), "text", false), nil
		})
	return nil
}

func (w *workspaceScreens) ReadWorkspaceFile(argument string) error {
	selection, err := parseWorkspaceReadSelection(argument)
	if err != nil {
		return err
	}
	request := workspace.ReadRequest{
		Workspace: w.session.current.Workspace.Path, Path: selection.path,
		Range: selection.lineRange, ByteLimit: selection.byteLimit,
	}
	w.runWorkspaceQuery("reading workspace file",
		func(ctx context.Context) (readerDocument, error) {
			content, err := w.service.Read(ctx, request)
			if err != nil {
				return readerDocument{}, err
			}
			detail := content.Window()
			if content.Truncated {
				detail += " · truncated"
			}
			return codeDocument("Workspace file", detail, content.Content, request.Path, true), nil
		})
	return nil
}

func (w *workspaceScreens) runWorkspaceQuery(status string, read func(context.Context) (readerDocument, error)) {
	w.reader.show(&readerQuery{status: status, read: read})
}

func paragraphDocument(title, detail string, lines []string) readerDocument {
	text := strings.Join(lines, "\n")
	if strings.TrimSpace(text) == "" {
		text = "No results."
	}
	return readerDocument{Title: title, Detail: detail, Sections: []toolSection{{Title: "Results", Style: toolSectionParagraph, Text: text}}}
}

func codeDocument(title, detail, text, pathOrLanguage string, lineNumbers bool) readerDocument {
	language := pathOrLanguage
	if strings.ContainsAny(pathOrLanguage, `./\`) {
		language = languageForPath(pathOrLanguage)
	}
	return readerDocument{Title: title, Detail: detail, Sections: []toolSection{{Title: "Content", Style: toolSectionCode, Language: language, Text: text, LineNumbers: lineNumbers}}}
}

func workspaceChangesDocument(path string, changes []workspace.Change) readerDocument {
	ordered := append([]workspace.Change(nil), changes...)
	sort.Slice(ordered, func(left, right int) bool { return ordered[left].Path < ordered[right].Path })
	lines := make([]string, 0, len(ordered))
	for _, change := range ordered {
		pathLabel := change.Path
		if change.PreviousPath != "" {
			pathLabel = change.PreviousPath + " → " + change.Path
		}
		stat := change.Stat()
		if stat != "" {
			stat = "  " + stat
		}
		lines = append(lines, fmt.Sprintf("%-10s %s%s", change.Status, pathLabel, stat))
	}
	return paragraphDocument("Workspace changes", fmt.Sprintf("%d files · %s", len(changes), path), lines)
}

// showChanges updates the open changes read with changes the monitor observed.
func (w *workspaceScreens) showChanges(changes []workspace.Change) {
	w.reader.update(w.changesView, workspaceChangesDocument(w.session.current.Workspace.Path, changes))
}
