package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestConsumptionRequiresAnActualConnectionPortCall(t *testing.T) {
	root := t.TempDir()
	for name, source := range map[string]string{
		"apps/cli/internal/adapter/runtimebinding/schedules.go": `package runtimebinding
type scheduleBinding interface { ListCatalog(); Unused() }
type Connection struct { schedules scheduleBinding }
func (r *Connection) Schedules() { r.schedules.ListCatalog() }
func (r *Unrelated) Other() { r.schedules.Unused() }`,
		"apps/cli/internal/adapter/runtimebinding/schedules_test.go": `package runtimebinding; func (r *Connection) Test() { r.schedules.Unused() }`,
	} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
	}
	got := collect(root, map[string]string{"ListCatalog": "catalog.list", "Unused": "unused"})
	want := map[string][]string{"catalog.list": {"apps/cli/internal/adapter/runtimebinding/schedules.go:4"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("consumers = %v, want %v", got, want)
	}
}
