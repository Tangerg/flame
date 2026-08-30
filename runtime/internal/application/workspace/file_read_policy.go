package workspace

// HeadLineLimit is the caller's file-preview line-count intent. The zero value
// is the named default; explicit values are positive and clamp at the
// Application-owned preview maximum.
type HeadLineLimit struct {
	explicit bool
	lines    int
}

func DefaultHeadLineLimit() HeadLineLimit { return HeadLineLimit{} }

func NewHeadLineLimit(lines int) (HeadLineLimit, error) {
	if lines <= 0 {
		return HeadLineLimit{}, ErrInvalidFileRange
	}
	return HeadLineLimit{explicit: true, lines: lines}, nil
}

func (l HeadLineLimit) Lines() (int, error) {
	if !l.explicit {
		if l.lines != 0 {
			return 0, ErrInvalidFileRange
		}
		return defaultFileHeadLines, nil
	}
	if l.lines <= 0 {
		return 0, ErrInvalidFileRange
	}
	return min(l.lines, maxFileHeadLines), nil
}

// GrepResultLimit is the caller's retained whole-match count. The search still
// computes its honest Total; this value only bounds the stable prefix returned.
type GrepResultLimit struct {
	explicit bool
	matches  int
}

func DefaultGrepResultLimit() GrepResultLimit { return GrepResultLimit{} }

func NewGrepResultLimit(matches int) (GrepResultLimit, error) {
	if matches <= 0 {
		return GrepResultLimit{}, ErrInvalidGrepLimit
	}
	return GrepResultLimit{explicit: true, matches: matches}, nil
}

func (l GrepResultLimit) Matches() (int, error) {
	if !l.explicit {
		if l.matches != 0 {
			return 0, ErrInvalidGrepLimit
		}
		return DefaultGrepLimit, nil
	}
	if l.matches <= 0 {
		return 0, ErrInvalidGrepLimit
	}
	return min(l.matches, MaxGrepLimit), nil
}

// FileReadByteLimit is the caller's retained UTF-8 byte budget. The zero value
// asks for the named default; explicit budgets are positive and clamp at the
// Application-owned response maximum.
type FileReadByteLimit struct {
	explicit bool
	bytes    int
}

func DefaultFileReadByteLimit() FileReadByteLimit { return FileReadByteLimit{} }

func NewFileReadByteLimit(bytes int) (FileReadByteLimit, error) {
	if bytes <= 0 {
		return FileReadByteLimit{}, ErrInvalidFileReadLimit
	}
	return FileReadByteLimit{explicit: true, bytes: bytes}, nil
}

func (l FileReadByteLimit) Bytes() (int, error) {
	if !l.explicit {
		if l.bytes != 0 {
			return 0, ErrInvalidFileReadLimit
		}
		return DefaultFileReadBytes, nil
	}
	if l.bytes <= 0 {
		return 0, ErrInvalidFileReadLimit
	}
	return min(l.bytes, MaxFileReadBytes), nil
}

// FileLineRange is a closed one-based inclusive read window. Its zero value is
// the whole file. A tail window has only Start; a bounded window has Start and
// End. End can never exist without Start.
type FileLineRange struct {
	kind  fileLineRangeKind
	start int
	end   int
}

type fileLineRangeKind uint8

const (
	fileLineRangeWhole fileLineRangeKind = iota
	fileLineRangeTail
	fileLineRangeBounded
)

func WholeFileRange() FileLineRange { return FileLineRange{kind: fileLineRangeWhole} }

func NewFileTailRange(start int) (FileLineRange, error) {
	if start <= 0 {
		return FileLineRange{}, ErrInvalidFileRange
	}
	return FileLineRange{kind: fileLineRangeTail, start: start}, nil
}

func NewFileLineRange(start, end int) (FileLineRange, error) {
	if start <= 0 || end < start {
		return FileLineRange{}, ErrInvalidFileRange
	}
	return FileLineRange{kind: fileLineRangeBounded, start: start, end: end}, nil
}

// Bounds returns the normalized filesystem-port coordinates. Zero/zero means
// the whole file only after the closed range has validated its own state.
func (r FileLineRange) Bounds() (start, end int, err error) {
	switch r.kind {
	case fileLineRangeWhole:
		if r.start != 0 || r.end != 0 {
			return 0, 0, ErrInvalidFileRange
		}
		return 0, 0, nil
	case fileLineRangeTail:
		if r.start <= 0 || r.end != 0 {
			return 0, 0, ErrInvalidFileRange
		}
		return r.start, 0, nil
	case fileLineRangeBounded:
		if r.start <= 0 || r.end < r.start {
			return 0, 0, ErrInvalidFileRange
		}
		return r.start, r.end, nil
	default:
		return 0, 0, ErrInvalidFileRange
	}
}
