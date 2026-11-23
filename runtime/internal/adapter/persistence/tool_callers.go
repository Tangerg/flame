package persistence

import (
	"context"
	"database/sql"
	json "encoding/json/v2"
	"errors"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"

	framecall "github.com/Tangerg/frame/call"
	framevalue "github.com/Tangerg/frame/value"
)

type storedToolCaller struct {
	Invocation framecall.Invocation `json:"invocation"`
	Tool       tool.Ref             `json:"tool"`
	Release    framevalue.Digest    `json:"release"`
	Authority  framevalue.Digest    `json:"authority"`
}

func storeToolCaller(value plugin.ToolCallerSnapshot) storedToolCaller {
	return storedToolCaller{Invocation: value.Invocation, Tool: value.Tool, Release: value.Release, Authority: value.Authority}
}

func (s storedToolCaller) snapshot() plugin.ToolCallerSnapshot {
	return plugin.ToolCallerSnapshot{Invocation: s.Invocation, Tool: s.Tool, Release: s.Release, Authority: s.Authority}
}

func (t *ToolInvocationStore) retainCaller(ctx context.Context, caller plugin.ToolCaller) error {
	return t.db.write(ctx, func(ctx context.Context) error {
		if err := caller.Validate(); err != nil {
			return err
		}
		value := caller.Snapshot()
		encoded, err := json.Marshal(storeToolCaller(value))
		if err != nil {
			return err
		}
		_, err = t.db.connection(ctx).ExecContext(ctx, `INSERT INTO installed_tool_callers(call_id,caller_json) VALUES(?,?) ON CONFLICT(call_id) DO NOTHING`, value.Invocation.ID, string(encoded))
		if err != nil {
			return err
		}
		retained, found, err := t.ToolCaller(ctx, value.Invocation.ID)
		if err != nil {
			return err
		}
		if !found || retained.Snapshot() != value {
			return plugin.ErrCommandConflict
		}
		return nil
	})
}

func (t *ToolInvocationStore) ToolCaller(ctx context.Context, id string) (plugin.ToolCaller, bool, error) {
	return scopedPair(ctx, t.db.read, func(ctx context.Context) (plugin.ToolCaller, bool, error) {
		var encoded string
		err := t.db.connection(ctx).QueryRowContext(ctx, `SELECT caller_json FROM installed_tool_callers WHERE call_id=?`, id).Scan(&encoded)
		if errors.Is(err, sql.ErrNoRows) {
			return plugin.ToolCaller{}, false, nil
		}
		if err != nil {
			return plugin.ToolCaller{}, false, err
		}
		var stored storedToolCaller
		if err := json.Unmarshal([]byte(encoded), &stored, json.RejectUnknownMembers(true)); err != nil {
			return plugin.ToolCaller{}, false, err
		}
		value := stored.snapshot()
		caller, err := plugin.NewToolCaller(value)
		if err != nil {
			return plugin.ToolCaller{}, false, err
		}
		if value.Invocation.ID != id {
			return plugin.ToolCaller{}, false, plugin.ErrCommandConflict
		}
		return caller, true, nil
	})
}
