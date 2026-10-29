package workbench

import (
	"errors"
	"fmt"
	"io"
	"reflect"

	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
)

// WriteError retains the outcome of this write attempt. A later write refused
// by an existing barrier is uncommitted even when its cause contains an earlier
// committed failure.
type WriteError struct {
	Record string
	Result filesystem.CommitResult
}

func (e *WriteError) Error() string {
	if e.Result.Committed() {
		return fmt.Sprintf("workbench record %q is committed: %v", e.Record, e.Result.Err)
	}
	return fmt.Sprintf("workbench record %q was not committed: %v", e.Record, e.Result.Err)
}

func (e *WriteError) Unwrap() error { return e.Result.Err }

func writeCommitted(err error) bool {
	write, ok := errors.AsType[*WriteError](err)
	return ok && write.Result.Committed()
}

// Persistence is the filesystem-neutral port used by Store. Workbench owns
// record names, formats, and recovery semantics; an external adapter owns the
// physical root and atomic file operations.
type Persistence interface {
	Read(name string, maximumBytes int64) ([]byte, error)
	ListFiles(directory, extension string) ([]string, error)
	Replace(name string, body []byte) filesystem.CommitResult
	Remove(name string) filesystem.CommitResult
	ConfirmDurability() error
}

func closePersistence(storage Persistence) error {
	if closer, ok := storage.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

func missingPersistence(storage Persistence) bool {
	if storage == nil {
		return true
	}
	value := reflect.ValueOf(storage)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
