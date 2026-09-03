package delivery

import (
	"context"
	"fmt"

	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
	"github.com/Tangerg/flame/runtime/protocol"
)

// ListWorkspaceFiles projects a paged application workspace-file listing onto
// the wire contract.
func (s *Handler) ListWorkspaceFiles(ctx context.Context, in protocol.ListFilesRequest) (*protocol.Page[protocol.FileEntry], error) {
	limit, err := requestedPageLimit(in.Limit)
	if err != nil {
		return nil, wireWorkspaceError(wirePageError(err))
	}
	page, err := s.workspaceFiles.List(ctx, workspaceapp.FileListInput{
		CWD: in.Workspace.Path,
		FileListOptions: workspaceapp.FileListOptions{
			Path: in.Path, Glob: in.Glob, Recursive: in.Recursive, IncludeIgnored: in.IncludeIgnored,
		},
		Cursor: in.Cursor,
		Limit:  limit,
	})
	if err != nil {
		return nil, wireWorkspaceError(err)
	}
	data := make([]protocol.FileEntry, 0, len(page.Entries))
	for _, entry := range page.Entries {
		kind, ok := presentFileEntryType(entry.Kind)
		if !ok {
			return nil, fmt.Errorf("workspace.files.list: unsupported entry kind %q", entry.Kind)
		}
		var sizeBytes *int64
		if entry.Kind == workspaceapp.FileEntryFile {
			sizeBytes = &entry.SizeBytes
		}
		data = append(data, protocol.FileEntry{
			Path: entry.Path, Name: entry.Name, Type: kind, SizeBytes: sizeBytes,
			ModifiedAt: entry.ModifiedAt,
		})
	}
	return protocol.NewPageWithCursor(data, page.NextCursor), nil
}

func presentFileEntryType(kind workspaceapp.FileEntryKind) (protocol.FileEntryType, bool) {
	switch kind {
	case workspaceapp.FileEntryFile:
		return protocol.FileEntryFile, true
	case workspaceapp.FileEntryDir:
		return protocol.FileEntryDir, true
	case workspaceapp.FileEntrySymlink:
		return protocol.FileEntrySymlink, true
	default:
		return "", false
	}
}

// GetWorkspaceFileHead projects the application file preview onto wire lines.
func (s *Handler) GetWorkspaceFileHead(ctx context.Context, in protocol.GetFileHeadRequest) (*protocol.FileHead, error) {
	lineLimit := workspaceapp.DefaultHeadLineLimit()
	if in.Lines != nil {
		explicit, err := workspaceapp.NewHeadLineLimit(*in.Lines)
		if err != nil {
			return nil, wireWorkspaceError(err)
		}
		lineLimit = explicit
	}
	head, err := s.workspaceFiles.Head(ctx, in.Workspace.Path, in.Path, lineLimit)
	if err != nil {
		return nil, wireWorkspaceError(err)
	}
	lines := make([]protocol.FileLine, 0, len(head.Lines))
	for _, line := range head.Lines {
		lines = append(lines, protocol.FileLine{LineNumber: line.Number, Text: line.Text})
	}
	return &protocol.FileHead{Path: in.Path, Lines: lines}, nil
}

// ReadWorkspaceFile maps the application file read onto the protocol response.
func (s *Handler) ReadWorkspaceFile(ctx context.Context, in protocol.ReadFileRequest) (*protocol.FileContent, error) {
	lineRange, err := fileLineRangeFromWire(in.StartLine, in.EndLine)
	if err != nil {
		return nil, wireWorkspaceError(err)
	}
	byteLimit := workspaceapp.DefaultFileReadByteLimit()
	if in.MaxBytes != nil {
		explicit, limitErr := workspaceapp.NewFileReadByteLimit(*in.MaxBytes)
		if limitErr != nil {
			return nil, wireWorkspaceError(limitErr)
		}
		byteLimit = explicit
	}
	read, err := s.workspaceFiles.Read(ctx, in.Workspace.Path, workspaceapp.FileReadInput{
		Path: in.Path, Range: lineRange, ByteLimit: byteLimit,
	})
	if err != nil {
		return nil, wireWorkspaceError(err)
	}
	out := &protocol.FileContent{
		Content: read.Content, TotalLines: read.TotalLines, Truncated: read.Truncated,
	}
	if in.StartLine != nil && read.EndLine > read.StartLine {
		out.StartLine = read.StartLine + 1
		out.EndLine = read.EndLine
	}
	return out, nil
}

// GrepWorkspace maps the application content search onto the protocol result.
func (s *Handler) GrepWorkspace(ctx context.Context, in protocol.GrepRequest) (*protocol.GrepResult, error) {
	limit := workspaceapp.DefaultGrepResultLimit()
	if in.Limit != nil {
		explicit, err := workspaceapp.NewGrepResultLimit(*in.Limit)
		if err != nil {
			return nil, wireWorkspaceError(err)
		}
		limit = explicit
	}
	result, err := s.workspaceFiles.Grep(ctx, in.Workspace.Path, workspaceapp.GrepInput{Path: in.Path, Query: in.Query, Limit: limit})
	if err != nil {
		return nil, wireWorkspaceError(err)
	}
	matches := make([]protocol.GrepMatch, 0, len(result.Matches))
	for _, match := range result.Matches {
		matches = append(matches, protocol.GrepMatch{Path: match.Path, LineNumber: match.LineNumber, Text: match.Text})
	}
	return &protocol.GrepResult{Matches: matches, Total: result.Total}, nil
}

func fileLineRangeFromWire(start, end *int) (workspaceapp.FileLineRange, error) {
	switch {
	case start == nil && end == nil:
		return workspaceapp.WholeFileRange(), nil
	case start != nil && end == nil:
		return workspaceapp.NewFileTailRange(*start)
	case start != nil && end != nil:
		return workspaceapp.NewFileLineRange(*start, *end)
	default:
		return workspaceapp.FileLineRange{}, workspaceapp.ErrInvalidFileRange
	}
}
