//go:build !darwin

package main

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	framemcp "github.com/Tangerg/frame/process/mcp"
	framevalue "github.com/Tangerg/frame/value"

	rpc "github.com/Tangerg/flame/plugins/sandbox/internal/contract"
	"github.com/Tangerg/flame/plugins/sandbox/internal/wire"
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
	input := wire.ExecuteInput{Script: "exit 0", TimeoutMillis: 1000}
	encoded, _ := framevalue.EncodeJSON(input)
	for range 2 {
		_, err := runner.execute(t.Context(), contracts.Execute.Name(), "unavailable", encoded)
		refusal, known := errors.AsType[*framemcp.Refusal](err)
		if !known || refusal.Code != "sandbox_unavailable" {
			t.Fatalf("unsupported confinement was not refused: %v", err)
		}
	}
}
