package terminal

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/Tangerg/flame/cli/internal/domain/workspace"
)

type commandOptionCursor struct {
	remaining string
	seen      map[string]struct{}
	stopped   bool
}

func newCommandOptionCursor(argument string) *commandOptionCursor {
	return &commandOptionCursor{remaining: strings.TrimSpace(argument), seen: make(map[string]struct{})}
}

func (c *commandOptionCursor) Next() (string, bool, error) {
	if c.stopped || c.remaining == "" {
		return "", false, nil
	}
	token, rest := nextCommandToken(c.remaining)
	if token == "--" {
		c.remaining, c.stopped = strings.TrimSpace(rest), true
		return "", false, nil
	}
	if !strings.HasPrefix(token, "--") {
		return "", false, nil
	}
	c.remaining = strings.TrimSpace(rest)
	if _, duplicate := c.seen[token]; duplicate {
		return "", false, fmt.Errorf("option %s was specified more than once", token)
	}
	c.seen[token] = struct{}{}
	return token, true, nil
}

func (c *commandOptionCursor) Value(option string) (string, error) {
	value, rest := nextCommandToken(c.remaining)
	if value == "" || strings.HasPrefix(value, "--") {
		return "", fmt.Errorf("option %s requires a value", option)
	}
	c.remaining = strings.TrimSpace(rest)
	return value, nil
}

func (c *commandOptionCursor) PositiveInt(option string) (int, error) {
	value, err := c.Value(option)
	if err != nil {
		return 0, err
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("option %s requires a positive integer", option)
	}
	return parsed, nil
}

func (c *commandOptionCursor) Rest() string { return strings.TrimSpace(c.remaining) }

func nextCommandToken(value string) (string, string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", ""
	}
	if boundary := strings.IndexAny(value, " \t\r\n"); boundary >= 0 {
		return value[:boundary], value[boundary:]
	}
	return value, ""
}

type workspaceDiffSelection struct {
	path   string
	mode   workspace.DiffMode
	format workspace.DiffFormat
	limit  workspace.DiffRowLimit
}

func parseWorkspaceDiffSelection(argument string) (workspaceDiffSelection, error) {
	selection := workspaceDiffSelection{
		mode: workspace.DiffModeWorktree, format: workspace.DiffFormatRaw,
		limit: workspace.DefaultDiffRowLimit(),
	}
	cursor := newCommandOptionCursor(argument)
	var modeSet, formatSet bool
	for {
		option, ok, err := cursor.Next()
		if err != nil {
			return workspaceDiffSelection{}, err
		}
		if !ok {
			break
		}
		switch option {
		case "--base", "--worktree":
			if modeSet {
				return workspaceDiffSelection{}, errors.New("workspace diff mode was specified more than once")
			}
			modeSet = true
			if option == "--base" {
				selection.mode = workspace.DiffModeBase
			}
		case "--rows", "--raw":
			if formatSet {
				return workspaceDiffSelection{}, errors.New("workspace diff format was specified more than once")
			}
			formatSet = true
			if option == "--rows" {
				selection.format = workspace.DiffFormatRows
			}
		case "--limit":
			rows, parseErr := cursor.PositiveInt(option)
			if parseErr != nil {
				return workspaceDiffSelection{}, parseErr
			}
			selection.limit, err = workspace.NewDiffRowLimit(rows)
			if err != nil {
				return workspaceDiffSelection{}, err
			}
		default:
			return workspaceDiffSelection{}, fmt.Errorf("unknown workspace diff option %q", option)
		}
	}
	selection.path = cursor.Rest()
	_, explicit, err := selection.limit.Rows()
	if err != nil {
		return workspaceDiffSelection{}, err
	}
	if explicit && selection.format != workspace.DiffFormatRows {
		return workspaceDiffSelection{}, errors.New("workspace diff --limit requires --rows")
	}
	return selection, nil
}

type workspaceHeadSelection struct {
	path  string
	lines workspace.HeadLineLimit
}

func parseWorkspaceHeadSelection(argument string) (workspaceHeadSelection, error) {
	selection := workspaceHeadSelection{lines: workspace.DefaultHeadLineLimit()}
	cursor := newCommandOptionCursor(argument)
	for {
		option, ok, err := cursor.Next()
		if err != nil {
			return workspaceHeadSelection{}, err
		}
		if !ok {
			break
		}
		if option != "--lines" {
			return workspaceHeadSelection{}, fmt.Errorf("unknown file preview option %q", option)
		}
		lines, parseErr := cursor.PositiveInt(option)
		if parseErr != nil {
			return workspaceHeadSelection{}, parseErr
		}
		selection.lines, err = workspace.NewHeadLineLimit(lines)
		if err != nil {
			return workspaceHeadSelection{}, err
		}
	}
	selection.path = cursor.Rest()
	if selection.path == "" {
		return workspaceHeadSelection{}, errors.New("usage: /preview [--lines N] <path>")
	}
	return selection, nil
}

type workspaceSearchSelection struct {
	query string
	path  string
	limit workspace.SearchResultLimit
}

func parseWorkspaceSearchSelection(argument string) (workspaceSearchSelection, error) {
	selection := workspaceSearchSelection{limit: workspace.DefaultSearchResultLimit()}
	cursor := newCommandOptionCursor(argument)
	for {
		option, ok, err := cursor.Next()
		if err != nil {
			return workspaceSearchSelection{}, err
		}
		if !ok {
			break
		}
		switch option {
		case "--path":
			selection.path, err = cursor.Value(option)
		case "--limit":
			matches, parseErr := cursor.PositiveInt(option)
			if parseErr != nil {
				return workspaceSearchSelection{}, parseErr
			}
			selection.limit, err = workspace.NewSearchResultLimit(matches)
		default:
			return workspaceSearchSelection{}, fmt.Errorf("unknown workspace search option %q", option)
		}
		if err != nil {
			return workspaceSearchSelection{}, err
		}
	}
	selection.query = cursor.Rest()
	if selection.query == "" {
		return workspaceSearchSelection{}, errors.New("usage: /grep [--path PATH] [--limit N] <query>")
	}
	return selection, nil
}

type workspaceFilesSelection struct {
	path           string
	glob           string
	recursive      bool
	includeIgnored bool
}

func parseWorkspaceFilesSelection(argument string) (workspaceFilesSelection, error) {
	selection := workspaceFilesSelection{}
	cursor := newCommandOptionCursor(argument)
	for {
		option, ok, err := cursor.Next()
		if err != nil {
			return workspaceFilesSelection{}, err
		}
		if !ok {
			break
		}
		switch option {
		case "--recursive":
			selection.recursive = true
		case "--ignored":
			selection.includeIgnored = true
		case "--glob":
			selection.glob, err = cursor.Value(option)
			if err != nil {
				return workspaceFilesSelection{}, err
			}
		default:
			return workspaceFilesSelection{}, fmt.Errorf("unknown workspace browse option %q", option)
		}
	}
	selection.path = cursor.Rest()
	return selection, nil
}

type workspaceReadSelection struct {
	path      string
	lineRange workspace.ReadLineRange
	byteLimit workspace.ReadByteLimit
}

func parseWorkspaceReadSelection(argument string) (workspaceReadSelection, error) {
	selection := workspaceReadSelection{
		lineRange: workspace.WholeFileReadRange(),
		byteLimit: workspace.DefaultReadByteLimit(),
	}
	var startLine, endLine *int
	cursor := newCommandOptionCursor(argument)
	for {
		option, ok, err := cursor.Next()
		if err != nil {
			return workspaceReadSelection{}, err
		}
		if !ok {
			break
		}
		switch option {
		case "--start":
			value, parseErr := cursor.PositiveInt(option)
			if parseErr != nil {
				return workspaceReadSelection{}, parseErr
			}
			startLine = &value
		case "--end":
			value, parseErr := cursor.PositiveInt(option)
			if parseErr != nil {
				return workspaceReadSelection{}, parseErr
			}
			endLine = &value
		case "--max-bytes":
			value, parseErr := cursor.PositiveInt(option)
			if parseErr != nil {
				return workspaceReadSelection{}, parseErr
			}
			selection.byteLimit, err = workspace.NewReadByteLimit(value)
		default:
			return workspaceReadSelection{}, fmt.Errorf("unknown workspace read option %q", option)
		}
		if err != nil {
			return workspaceReadSelection{}, err
		}
	}
	selection.path = cursor.Rest()
	var rangeErr error
	switch {
	case selection.path == "":
		return workspaceReadSelection{}, errors.New("usage: /read [--start N] [--end N] [--max-bytes N] <path>")
	case startLine == nil && endLine != nil:
		return workspaceReadSelection{}, errors.New("workspace read --end requires --start")
	case startLine != nil && endLine == nil:
		selection.lineRange, rangeErr = workspace.NewReadTailRange(*startLine)
	case startLine != nil && endLine != nil:
		selection.lineRange, rangeErr = workspace.NewReadLineRange(*startLine, *endLine)
	}
	if rangeErr != nil {
		return workspaceReadSelection{}, rangeErr
	}
	return selection, nil
}
