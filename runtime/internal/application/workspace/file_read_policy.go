package workspace

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
	return min(l.bytes, maxFileReadBytes)
}

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

func (r FileLineRange) Bounds() (start, end int) { return r.start, r.end }
