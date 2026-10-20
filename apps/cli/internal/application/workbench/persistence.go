package workbench

import (
	"io"
	"reflect"
)

// Persistence is the filesystem-neutral port used by Store. Workbench owns
// record names, formats, and recovery semantics; an external adapter owns the
// physical root and atomic file operations.
type Persistence interface {
	Read(name string, maximumBytes int64) ([]byte, error)
	ListFiles(directory, extension string) ([]string, error)
	Replace(name string, body []byte) error
	Remove(name string) error
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
