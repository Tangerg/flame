package workspace

import "errors"

const (
	DefaultHeadLines     = 80
	DefaultSearchMatches = 200
	DefaultReadBytes     = 2 << 20
)

type requestLimitKind uint8

const (
	defaultRequestLimit requestLimitKind = iota + 1
	explicitRequestLimit
)

type HeadLineLimit struct {
	kind  requestLimitKind
	lines int
}

func DefaultHeadLineLimit() HeadLineLimit { return HeadLineLimit{kind: defaultRequestLimit} }

func NewHeadLineLimit(lines int) (HeadLineLimit, error) {
	if lines <= 0 {
		return HeadLineLimit{}, errors.New("file head line count must be positive")
	}
	return HeadLineLimit{kind: explicitRequestLimit, lines: lines}, nil
}

func (l HeadLineLimit) Lines() (int, error) {
	switch l.kind {
	case defaultRequestLimit:
		if l.lines != 0 {
			return 0, errors.New("default file head line limit carries a value")
		}
		return DefaultHeadLines, nil
	case explicitRequestLimit:
		if l.lines <= 0 {
			return 0, errors.New("file head line count must be positive")
		}
		return l.lines, nil
	default:
		return 0, errors.New("file head line limit kind is unknown")
	}
}

type SearchResultLimit struct {
	kind    requestLimitKind
	matches int
}

func DefaultSearchResultLimit() SearchResultLimit {
	return SearchResultLimit{kind: defaultRequestLimit}
}

func NewSearchResultLimit(matches int) (SearchResultLimit, error) {
	if matches <= 0 {
		return SearchResultLimit{}, errors.New("workspace search result limit must be positive")
	}
	return SearchResultLimit{kind: explicitRequestLimit, matches: matches}, nil
}

func (l SearchResultLimit) Matches() (int, error) {
	switch l.kind {
	case defaultRequestLimit:
		if l.matches != 0 {
			return 0, errors.New("default workspace search result limit carries a value")
		}
		return DefaultSearchMatches, nil
	case explicitRequestLimit:
		if l.matches <= 0 {
			return 0, errors.New("workspace search result limit must be positive")
		}
		return l.matches, nil
	default:
		return 0, errors.New("workspace search result limit kind is unknown")
	}
}

type ReadByteLimit struct {
	kind  requestLimitKind
	bytes int
}

func DefaultReadByteLimit() ReadByteLimit { return ReadByteLimit{kind: defaultRequestLimit} }

func NewReadByteLimit(bytes int) (ReadByteLimit, error) {
	if bytes <= 0 {
		return ReadByteLimit{}, errors.New("workspace read byte limit must be positive")
	}
	return ReadByteLimit{kind: explicitRequestLimit, bytes: bytes}, nil
}

func (l ReadByteLimit) Bytes() (int, error) {
	switch l.kind {
	case defaultRequestLimit:
		if l.bytes != 0 {
			return 0, errors.New("default workspace read byte limit carries a value")
		}
		return DefaultReadBytes, nil
	case explicitRequestLimit:
		if l.bytes <= 0 {
			return 0, errors.New("workspace read byte limit must be positive")
		}
		return l.bytes, nil
	default:
		return 0, errors.New("workspace read byte limit kind is unknown")
	}
}

type ReadLineRange struct {
	kind  readLineRangeKind
	start int
	end   int
}

type readLineRangeKind uint8

const (
	readLineRangeWhole readLineRangeKind = iota + 1
	readLineRangeTail
	readLineRangeBounded
)

func WholeFileReadRange() ReadLineRange { return ReadLineRange{kind: readLineRangeWhole} }

func NewReadTailRange(start int) (ReadLineRange, error) {
	if start <= 0 {
		return ReadLineRange{}, errors.New("workspace read start line must be positive")
	}
	return ReadLineRange{kind: readLineRangeTail, start: start}, nil
}

func NewReadLineRange(start, end int) (ReadLineRange, error) {
	if start <= 0 || end < start {
		return ReadLineRange{}, errors.New("workspace read line range is invalid")
	}
	return ReadLineRange{kind: readLineRangeBounded, start: start, end: end}, nil
}

func (r ReadLineRange) Bounds() (start, end int, err error) {
	switch r.kind {
	case readLineRangeWhole:
		if r.start != 0 || r.end != 0 {
			return 0, 0, errors.New("whole-file read range carries bounds")
		}
		return 0, 0, nil
	case readLineRangeTail:
		if r.start <= 0 || r.end != 0 {
			return 0, 0, errors.New("workspace read tail range is invalid")
		}
		return r.start, 0, nil
	case readLineRangeBounded:
		if r.start <= 0 || r.end < r.start {
			return 0, 0, errors.New("workspace read line range is invalid")
		}
		return r.start, r.end, nil
	default:
		return 0, 0, errors.New("workspace read line range kind is unknown")
	}
}
