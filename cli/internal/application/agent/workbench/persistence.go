package workbench

// Persistence is the filesystem-neutral port used by Store. Workbench owns
// record names, formats, and recovery semantics; an external adapter owns the
// physical root and atomic file operations.
type Persistence interface {
	Read(name string, maximumBytes int64) ([]byte, error)
	List(directory string) ([]string, error)
	Replace(name string, body []byte) error
	Remove(name string) error
}
