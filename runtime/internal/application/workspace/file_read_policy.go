package workspace

// Each limit below keeps one count: zero is the named default, and its
// constructor admits only a positive count, so no other state is reachable.

// HeadLineLimit is the caller's file-preview line-count intent. Explicit values
// clamp at the Application-owned preview maximum.
type HeadLineLimit struct{ lines int }

func DefaultHeadLineLimit() HeadLineLimit { return HeadLineLimit{} }

func NewHeadLineLimit(lines int) (HeadLineLimit, error) {
	if lines <= 0 {
		return HeadLineLimit{}, ErrInvalidFileRange
	}
	return HeadLineLimit{lines: lines}, nil
}

func (l HeadLineLimit) Lines() int {
	if l.lines == 0 {
		return defaultFileHeadLines
	}
	return min(l.lines, maxFileHeadLines)
}

// GrepResultLimit is the caller's retained whole-match count. The search still
// computes its honest Total; this value only bounds the stable prefix returned.
type GrepResultLimit struct{ matches int }

func DefaultGrepResultLimit() GrepResultLimit { return GrepResultLimit{} }

func NewGrepResultLimit(matches int) (GrepResultLimit, error) {
	if matches <= 0 {
		return GrepResultLimit{}, ErrInvalidGrepLimit
	}
	return GrepResultLimit{matches: matches}, nil
}

func (l GrepResultLimit) Matches() int {
	if l.matches == 0 {
		return DefaultGrepLimit
	}
	return min(l.matches, MaxGrepLimit)
}

// FileReadByteLimit is the caller's retained UTF-8 byte budget. Explicit
// budgets clamp at the Application-owned response maximum.
type FileReadByteLimit struct{ bytes int }

func DefaultFileReadByteLimit() FileReadByteLimit { return FileReadByteLimit{} }

func NewFileReadByteLimit(bytes int) (FileReadByteLimit, error) {
	if bytes <= 0 {
		return FileReadByteLimit{}, ErrInvalidFileReadLimit
	}
	return FileReadByteLimit{bytes: bytes}, nil
}

func (l FileReadByteLimit) Bytes() int {
	if l.bytes == 0 {
		return DefaultFileReadBytes
	}
	return min(l.bytes, MaxFileReadBytes)
}

// FileLineRange is a one-based inclusive read window. Its zero value is the
// whole file. A tail window has only start; a bounded window has start and
// end. The constructors admit no other shape, so end never exists without start.
type FileLineRange struct {
	start int
	end   int
}

func WholeFileRange() FileLineRange { return FileLineRange{} }

func NewFileTailRange(start int) (FileLineRange, error) {
	if start <= 0 {
		return FileLineRange{}, ErrInvalidFileRange
	}
	return FileLineRange{start: start}, nil
}

func NewFileLineRange(start, end int) (FileLineRange, error) {
	if start <= 0 || end < start {
		return FileLineRange{}, ErrInvalidFileRange
	}
	return FileLineRange{start: start, end: end}, nil
}

// Bounds returns the filesystem-port coordinates; zero means unbounded.
func (r FileLineRange) Bounds() (start, end int) { return r.start, r.end }
