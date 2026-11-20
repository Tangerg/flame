package terminal

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/Tangerg/flame/apps/cli/internal/domain/workspace"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

// Workspace command options. A bare optionTerminator ends option parsing.
const (
	optionPrefix     = "--"
	optionTerminator = "--"
	baseOption       = "--base"
	worktreeOption   = "--worktree"
	rowsOption       = "--rows"
	rawOption        = "--raw"
	limitOption      = "--limit"
	linesOption      = "--lines"
	pathOption       = "--path"
	scanAllOption    = "--scan-all"
	recursiveOption  = "--recursive"
	ignoredOption    = "--ignored"
	globOption       = "--glob"
	startOption      = "--start"
	endOption        = "--end"
	maxBytesOption   = "--max-bytes"
)

const defaultPreviewLines = 80

// exclusiveOptions admits at most one option of a mutually exclusive group.
type exclusiveOptions struct {
	group  string
	chosen string
}

func (g *exclusiveOptions) choose(option string) error {
	if g.chosen != "" {
		return fmt.Errorf("workspace diff %s was specified more than once", g.group)
	}
	g.chosen = option
	return nil
}

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
	if token == optionTerminator {
		c.remaining, c.stopped = strings.TrimSpace(rest), true
		return "", false, nil
	}
	if !strings.HasPrefix(token, optionPrefix) {
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
	if value == "" || strings.HasPrefix(value, optionPrefix) {
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
	mode   protocol.DiffMode
	format protocol.DiffFormat
	limit  workspace.DiffRowLimit
}

func parseWorkspaceDiffSelection(argument string) (workspaceDiffSelection, error) {
	selection := workspaceDiffSelection{
		mode: protocol.DiffModeWorktree, format: protocol.DiffFormatRaw,
		limit: workspace.DefaultDiffRowLimit(),
	}
	cursor := newCommandOptionCursor(argument)
	mode := exclusiveOptions{group: "mode"}
	format := exclusiveOptions{group: "format"}
	for {
		option, ok, err := cursor.Next()
		if err != nil {
			return workspaceDiffSelection{}, err
		}
		if !ok {
			break
		}
		switch option {
		case baseOption, worktreeOption:
			if err := mode.choose(option); err != nil {
				return workspaceDiffSelection{}, err
			}
			if option == baseOption {
				selection.mode = protocol.DiffModeBase
			}
		case rowsOption, rawOption:
			if err := format.choose(option); err != nil {
				return workspaceDiffSelection{}, err
			}
			if option == rowsOption {
				selection.format = protocol.DiffFormatRows
			}
		case limitOption:
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
	if selection.limit.Rows() > 0 && selection.format != protocol.DiffFormatRows {
		return workspaceDiffSelection{}, errors.New("workspace diff --limit requires --rows")
	}
	return selection, nil
}

type workspacePreviewSelection struct {
	path  string
	lines int
}

func parseWorkspacePreviewSelection(argument string) (workspacePreviewSelection, error) {
	selection := workspacePreviewSelection{lines: defaultPreviewLines}
	cursor := newCommandOptionCursor(argument)
	for {
		option, ok, err := cursor.Next()
		if err != nil {
			return workspacePreviewSelection{}, err
		}
		if !ok {
			break
		}
		if option != linesOption {
			return workspacePreviewSelection{}, fmt.Errorf("unknown file preview option %q", option)
		}
		selection.lines, err = cursor.PositiveInt(option)
		if err != nil {
			return workspacePreviewSelection{}, err
		}
	}
	selection.path = cursor.Rest()
	if selection.path == "" {
		return workspacePreviewSelection{}, errors.New("usage: /preview [--lines N] <path>")
	}
	return selection, nil
}

type workspaceSearchSelection struct {
	query   string
	path    string
	limit   workspace.SearchResultLimit
	scanAll bool
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
		case pathOption:
			selection.path, err = cursor.Value(option)
		case scanAllOption:
			selection.scanAll = true
		case limitOption:
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
		return workspaceSearchSelection{}, errors.New("usage: /grep [--path PATH] [--limit N] [--scan-all] <query>")
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
		case recursiveOption:
			selection.recursive = true
		case ignoredOption:
			selection.includeIgnored = true
		case globOption:
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
	cursor := newCommandOptionCursor(argument)
	startLine, endLine, err := parseWorkspaceReadOptions(cursor, &selection)
	if err != nil {
		return workspaceReadSelection{}, err
	}
	selection.path = cursor.Rest()
	if selection.path == "" {
		return workspaceReadSelection{}, errors.New("usage: /read [--start N] [--end N] [--max-bytes N] <path>")
	}
	switch {
	case startLine == nil && endLine != nil:
		return workspaceReadSelection{}, errors.New("workspace read --end requires --start")
	case startLine != nil && endLine == nil:
		selection.lineRange, err = workspace.NewReadTailRange(*startLine)
	case startLine != nil && endLine != nil:
		selection.lineRange, err = workspace.NewReadLineRange(*startLine, *endLine)
	}
	if err != nil {
		return workspaceReadSelection{}, err
	}
	return selection, nil
}

// parseWorkspaceReadOptions consumes the leading options; the line bounds
// stay separate because only their combination names a range.
func parseWorkspaceReadOptions(cursor *commandOptionCursor, selection *workspaceReadSelection) (startLine, endLine *int, err error) {
	for {
		option, ok, err := cursor.Next()
		if err != nil || !ok {
			return startLine, endLine, err
		}
		value, err := cursor.PositiveInt(option)
		switch {
		case option != startOption && option != endOption && option != maxBytesOption:
			return nil, nil, fmt.Errorf("unknown workspace read option %q", option)
		case err != nil:
			return nil, nil, err
		case option == startOption:
			startLine = &value
		case option == endOption:
			endLine = &value
		default:
			if selection.byteLimit, err = workspace.NewReadByteLimit(value); err != nil {
				return nil, nil, err
			}
		}
	}
}
