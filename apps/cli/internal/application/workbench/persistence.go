package workbench

type RecordChange struct {
	Key  string
	Body []byte // Encoded records are nonempty, leaving nil for removal.
}

type Persistence interface {
	Read(key string, maximumBytes int64) ([]byte, error)
	Keys(prefix string) ([]string, error)
	Commit(changes ...RecordChange) error
	Close() error
}
