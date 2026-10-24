package workspace

import "errors"

// SearchResultLimit preserves an explicit positive request. Zero leaves the
// default and material limits with Runtime.
type SearchResultLimit struct{ matches int }

func DefaultSearchResultLimit() SearchResultLimit { return SearchResultLimit{} }

func NewSearchResultLimit(matches int) (SearchResultLimit, error) {
	if matches <= 0 {
		return SearchResultLimit{}, errors.New("workspace search result limit must be positive")
	}
	return SearchResultLimit{matches: matches}, nil
}

func (l SearchResultLimit) Matches() int { return l.matches }

// ReadByteLimit preserves an explicit positive request. Zero leaves the byte
// budget with Runtime, including when a preview selects a line window.
type ReadByteLimit struct{ bytes int }

func DefaultReadByteLimit() ReadByteLimit { return ReadByteLimit{} }

func NewReadByteLimit(bytes int) (ReadByteLimit, error) {
	if bytes <= 0 {
		return ReadByteLimit{}, errors.New("workspace read byte limit must be positive")
	}
	return ReadByteLimit{bytes: bytes}, nil
}

func (l ReadByteLimit) Bytes() int { return l.bytes }

// ReadLineRange is whole-file when zero. A positive start selects a tail;
// a positive end bounds that window. Constructors own the ordering invariant.
type ReadLineRange struct {
	start int
	end   int
}

func WholeFileReadRange() ReadLineRange { return ReadLineRange{} }

func NewReadTailRange(start int) (ReadLineRange, error) {
	if start <= 0 {
		return ReadLineRange{}, errors.New("workspace read start line must be positive")
	}
	return ReadLineRange{start: start}, nil
}

func NewReadLineRange(start, end int) (ReadLineRange, error) {
	if start <= 0 || end < start {
		return ReadLineRange{}, errors.New("workspace read line range is invalid")
	}
	return ReadLineRange{start: start, end: end}, nil
}

func (r ReadLineRange) Bounds() (start, end int) { return r.start, r.end }
