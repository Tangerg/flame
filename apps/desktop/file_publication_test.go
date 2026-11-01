package main

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/iotest"
)

func TestExportPreservesPriorFileOnPartialWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "report")
	if err := os.WriteFile(path, []byte("original"), 0640); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("source read failed")
	if err := publishExport(path, io.MultiReader(strings.NewReader("prefix"), iotest.ErrReader(failure))); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if b, e := os.ReadFile(path); e != nil || string(b) != "original" {
		t.Fatalf("after failed save %q, %v", b, e)
	}
	if err := publishExport(path, strings.NewReader("replacement")); err != nil {
		t.Fatal(err)
	}
	if b, e := os.ReadFile(path); e != nil || string(b) != "replacement" {
		t.Fatalf("after successful save %q, %v", b, e)
	}
	if i, e := os.Stat(path); e != nil || i.Mode().Perm() != 0640 {
		t.Fatal(i, e)
	}
}
