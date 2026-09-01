package terminal

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Tangerg/flame/cli/internal/domain/workspace"
)

type workspaceReaderMode uint8

const (
	workspaceReaderNone workspaceReaderMode = iota
	workspaceReaderChanges
)

func (a *app) ShowWorkspaces() {
	a.runWorkspaceQuery("loading workspaces",
		func(ctx context.Context) (readerDocument, error) {
			values, err := a.workspaces.List(ctx)
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
		}, workspaceReaderNone)
}

func (a *app) ShowWorkspaceChanges() {
	path := a.session.current.Workspace.Path
	a.runWorkspaceQuery("loading workspace changes",
		func(ctx context.Context) (readerDocument, error) {
			changes, err := a.workspaces.Changes(ctx, path)
			if err != nil {
				return readerDocument{}, err
			}
			return workspaceChangesDocument(path, changes), nil
		}, workspaceReaderChanges)
}

func (a *app) ShowWorkspaceDiff(argument string) error {
	selection, err := parseWorkspaceDiffSelection(argument)
	if err != nil {
		return err
	}
	request := workspace.DiffRequest{
		Workspace: a.session.current.Workspace.Path, Path: selection.path,
		Mode: selection.mode, Format: selection.format, RowLimit: selection.limit,
	}
	a.runWorkspaceQuery("loading workspace diff",
		func(ctx context.Context) (readerDocument, error) {
			diff, err := a.workspaces.Diff(ctx, request)
			if err != nil {
				return readerDocument{}, err
			}
			text := diff.Text()
			if strings.TrimSpace(text) == "" {
				text = "No workspace differences."
			}
			detail := string(request.Mode) + " · " + string(request.Format)
			if request.Path != "" {
				detail += " · " + request.Path
			}
			if diff.Truncated {
				detail += " · truncated"
			}
			return readerDocument{Title: "Workspace diff", Detail: detail, Sections: []ToolSection{{Title: "Changes", Style: toolSectionDiff, Language: "diff", Text: text}}}, nil
		}, workspaceReaderNone)
	return nil
}

func (a *app) PreviewWorkspaceFile(argument string) error {
	selection, err := parseWorkspaceHeadSelection(argument)
	if err != nil {
		return err
	}
	previewLines, err := selection.lines.Lines()
	if err != nil {
		return err
	}
	request := workspace.HeadRequest{Workspace: a.session.current.Workspace.Path, Path: selection.path, LineLimit: selection.lines}
	a.runWorkspaceQuery("loading file preview",
		func(ctx context.Context) (readerDocument, error) {
			head, err := a.workspaces.Head(ctx, request)
			if err != nil {
				return readerDocument{}, err
			}
			lines := make([]string, 0, len(head.Lines))
			for _, line := range head.Lines {
				lines = append(lines, line.Text)
			}
			detail := fmt.Sprintf("%s · up to %d lines", head.Path, previewLines)
			return codeDocument("File preview", detail, strings.Join(lines, "\n"), head.Path, true), nil
		}, workspaceReaderNone)
	return nil
}

func (a *app) SearchWorkspace(argument string) error {
	selection, err := parseWorkspaceSearchSelection(argument)
	if err != nil {
		return err
	}
	matchLimit, err := selection.limit.Matches()
	if err != nil {
		return err
	}
	request := workspace.SearchRequest{
		Workspace: a.session.current.Workspace.Path, Query: selection.query, Path: selection.path, Limit: selection.limit,
	}
	a.runWorkspaceQuery("searching workspace",
		func(ctx context.Context) (readerDocument, error) {
			result, err := a.workspaces.Search(ctx, request)
			if err != nil {
				return readerDocument{}, err
			}
			lines := make([]string, 0, len(result.Matches))
			for _, match := range result.Matches {
				lines = append(lines, fmt.Sprintf("%s:%d  %s", match.Path, match.Line, match.Text))
			}
			detail := fmt.Sprintf("%d/%d matches", len(result.Matches), result.Total)
			if request.Path != "" {
				detail += " · " + request.Path
			}
			detail += fmt.Sprintf(" · limit %d", matchLimit)
			return paragraphDocument("Workspace search · "+request.Query, detail, lines), nil
		}, workspaceReaderNone)
	return nil
}

func (a *app) BrowseWorkspace(argument string) error {
	selection, err := parseWorkspaceFilesSelection(argument)
	if err != nil {
		return err
	}
	request := workspace.FilesRequest{
		Workspace: a.session.current.Workspace.Path, Path: selection.path, Glob: selection.glob,
		Recursive: selection.recursive, IncludeIgnored: selection.includeIgnored,
	}
	a.runWorkspaceQuery("browsing workspace",
		func(ctx context.Context) (readerDocument, error) {
			listing, err := a.workspaces.Files(ctx, request)
			if err != nil {
				return readerDocument{}, err
			}
			lines := make([]string, 0, len(listing.Entries))
			for _, entry := range listing.Entries {
				kind := string(entry.Type)
				if entry.Type == workspace.FileEntryDirectory {
					kind = "dir"
				}
				line := fmt.Sprintf("%-7s %s", kind, entry.Path)
				var metadata []string
				if entry.SizeBytes != nil {
					metadata = append(metadata, fmt.Sprintf("%d B", *entry.SizeBytes))
				}
				if entry.ModifiedAt != "" {
					metadata = append(metadata, entry.ModifiedAt)
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
		}, workspaceReaderNone)
	return nil
}

func (a *app) ReadWorkspaceFile(argument string) error {
	selection, err := parseWorkspaceReadSelection(argument)
	if err != nil {
		return err
	}
	request := workspace.ReadRequest{
		Workspace: a.session.current.Workspace.Path, Path: selection.path,
		Range: selection.lineRange, ByteLimit: selection.byteLimit,
	}
	a.runWorkspaceQuery("reading workspace file",
		func(ctx context.Context) (readerDocument, error) {
			content, err := a.workspaces.Read(ctx, request)
			if err != nil {
				return readerDocument{}, err
			}
			detail := content.Window()
			if content.Truncated {
				detail += " · truncated"
			}
			return codeDocument("Workspace file", detail, content.Content, content.Path, true), nil
		}, workspaceReaderNone)
	return nil
}

func (a *app) runWorkspaceQuery(status string, query func(context.Context) (readerDocument, error), mode workspaceReaderMode) {
	a.status.note(status)
	a.runOperation(readerDocumentOperation, true, query, func(document readerDocument, err error) {
		if err != nil {
			a.message("workspace: " + err.Error())
			return
		}
		a.dialogs.workspaceReader = mode
		a.setRuntimeReader(runtimeReaderNone)
		a.openReaderDocument(document)
		a.status.note(strings.ToLower(document.Title))
	})
}

func paragraphDocument(title, detail string, lines []string) readerDocument {
	text := strings.Join(lines, "\n")
	if strings.TrimSpace(text) == "" {
		text = "No results."
	}
	return readerDocument{Title: title, Detail: detail, Sections: []ToolSection{{Title: "Results", Style: toolSectionParagraph, Text: text}}}
}

func codeDocument(title, detail, text, pathOrLanguage string, lineNumbers bool) readerDocument {
	language := pathOrLanguage
	if strings.Contains(pathOrLanguage, ".") || strings.ContainsRune(pathOrLanguage, filepath.Separator) {
		language = languageForPath(pathOrLanguage)
	}
	return readerDocument{Title: title, Detail: detail, Sections: []ToolSection{{Title: "Content", Style: toolSectionCode, Language: language, Text: text, LineNumbers: lineNumbers}}}
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
