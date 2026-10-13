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
		"runtime/internal/delivery/methods.go": `package delivery; const SchedulesList Name = "schedules.list"; const Unused Name = "unused"`,
		"runtime/schedules.go": `package runtime
func (r *binding) ListSchedules() { r.invoke(delivery.SchedulesList) }
func (r *binding) Unused() { r.invoke(delivery.Unused) }`,
		"cli/internal/adapter/runtimebinding/schedules.go": `package runtimebinding
type scheduleBinding interface { ListSchedules(); Unused() }
type Connection struct { schedules scheduleBinding }
func (r *Connection) Schedules() { r.schedules.ListSchedules() }
func (r *Unrelated) Other() { r.schedules.Unused() }`,
		"cli/internal/adapter/runtimebinding/schedules_test.go": `package runtimebinding; func (r *Connection) Test() { r.schedules.Unused() }`,
	} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
	}
	got := collect(root)
	want := map[string][]string{"schedules.list": {"cli/internal/adapter/runtimebinding/schedules.go:4"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("consumers = %v, want %v", got, want)
	}
}
