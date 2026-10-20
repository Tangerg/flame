//go:build !darwin

package main

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/plugins/sandbox/backend/rpc"
	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/jsoninput"
)

func TestUnsupportedPlatformSettlesARefusalWithoutHostAccess(t *testing.T) {
	data := t.TempDir()
	db, err := openStore(t.Context(), filepath.Join(data, "sandbox.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	runner, err := newRunner(db, filepath.Join(data, "scratch"), func(context.Context) (string, error) { t.Error("unsupported platform accessed core"); return "", nil })
	if err != nil {
		t.Fatal(err)
	}
	contracts, _ := rpc.NewContracts()
	input := rpc.ExecuteInput{Script: "exit 0", TimeoutMillis: 1000}
	encoded, _ := jsoninput.Encode(input)
	for range 2 {
		_, err := runner.execute(t.Context(), contracts.Execute.Name(), "unavailable", input, encoded)
		refusal, known := errors.AsType[*capability.Refusal](err)
		if !known || refusal.Code != "sandbox_unavailable" {
			t.Fatalf("unsupported confinement was not refused: %v", err)
		}
	}
}
