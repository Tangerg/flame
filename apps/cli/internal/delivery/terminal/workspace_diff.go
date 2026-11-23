package terminal

import (
	"fmt"
	"strconv"

	"github.com/Tangerg/flame/apps/cli/internal/delivery/format"
	"github.com/Tangerg/flame/apps/cli/internal/domain/workspace"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/oolong/ui/core/text"
	"github.com/Tangerg/oolong/ui/kit"
)

func workspaceDiffDocument(request workspace.DiffRequest, diff workspace.Diff) readerDocument {
	detail := string(request.Mode) + format.Separator + string(request.Format)
	if request.Path != "" {
		detail += format.Separator + request.Path
	}
	if diff.Truncated {
		detail += format.Separator + "truncated"
	}
	var baseline string
	switch diff.Baseline.Type {
	case protocol.DiffBaselineHead:
		baseline = "HEAD " + diff.Baseline.Commit + " → working tree, including untracked files"
	case protocol.DiffBaselineMergeBase:
		baseline = "Merge base " + diff.Baseline.Commit + " → working tree, tracked files only"
	case protocol.DiffBaselineEmptyTree:
		baseline = "Empty tree → working tree, including untracked files"
	}
	document := readerDocument{Title: "Workspace diff", Detail: detail, Sections: []toolSection{{Title: "Comparison", Style: toolSectionParagraph, Text: baseline}}}
	if request.Format == protocol.DiffFormatRaw {
		if diff.Patch != "" {
			document.Sections = append(document.Sections, toolSection{Title: "Changes", Style: toolSectionCode, Language: codeLanguageDiff, Text: diff.Patch})
		}
	} else {
		for _, file := range diff.Files {
			document.Sections = append(document.Sections, fileDiffSection(file))
		}
	}
	if len(document.Sections) == 1 {
		message := "No workspace differences."
		if diff.Truncated {
			message = "No complete file fits the requested row limit."
		}
		document.Sections = append(document.Sections, toolSection{Style: toolSectionParagraph, Text: message})
	}
	return document
}

func renderDiffRows(theme kit.Theme, rows []protocol.DiffRow) *kit.Code {
	width := 1
	for _, row := range rows {
		width = max(width, len(strconv.Itoa(row.LeftLine)), len(strconv.Itoa(row.RightLine)))
	}
	lineNumber := func(number int) string {
		if number == 0 {
			return ""
		}
		return strconv.Itoa(number)
	}
	lines := make([]text.Line, 0, len(rows))
	for _, row := range rows {
		if row.Type == protocol.DiffRowHunk {
			lines = append(lines, text.Line{{Text: row.Text, Style: theme.Subtle}})
			continue
		}
		marker, style := " ", theme.Text
		switch row.Type {
		case protocol.DiffRowAdded:
			marker, style = "+", theme.Added
		case protocol.DiffRowDeleted:
			marker, style = "-", theme.Removed
		case protocol.DiffRowHunk, protocol.DiffRowContext:
		}
		lines = append(lines, text.Line{
			{Text: fmt.Sprintf("%*s %*s ", width, lineNumber(row.LeftLine), width, lineNumber(row.RightLine)), Style: theme.Subtle},
			{Text: marker + *row.Code, Style: style},
		})
		if row.NoNewline {
			lines = append(lines, text.Line{{Text: `\ No newline at end of file`, Style: theme.Subtle}})
		}
	}
	return kit.NewCode(lines)
}

func fileDiffSection(file workspace.FileDiff) toolSection {
	title := strconv.Quote(file.Path)
	if file.PreviousPath != "" {
		title = strconv.Quote(file.PreviousPath) + " → " + title
	}
	title += format.Separator + string(file.Status)
	if stat := file.Stat(); stat != "" {
		title += format.Separator + stat
	}
	if len(file.Rows) > 0 {
		return toolSection{Title: title, Style: toolSectionDiffRows, Rows: file.Rows}
	}
	text := "No text changes."
	if file.Binary {
		text = "Binary file."
	}
	return toolSection{Title: title, Style: toolSectionParagraph, Text: text}
}
