package persistence

import (
	"context"
	"database/sql"
	json "encoding/json/v2"
	"errors"

	"github.com/Tangerg/flame/packages/go-sdk/sqlite"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
)

func (t *ToolInvocationStore) retainCaller(ctx context.Context, caller plugin.ToolCaller) error {
	if err := caller.Validate(); err != nil {
		return err
	}
	value := caller.Snapshot()
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = sqlite.Connection(ctx, t.db).ExecContext(ctx, `INSERT INTO installed_tool_callers(call_id,caller_json) VALUES(?,?) ON CONFLICT(call_id) DO NOTHING`, value.Invocation.ID, string(encoded))
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
}

func (t *ToolInvocationStore) ToolCaller(ctx context.Context, id string) (plugin.ToolCaller, bool, error) {
	var encoded string
	err := sqlite.Connection(ctx, t.db).QueryRowContext(ctx, `SELECT caller_json FROM installed_tool_callers WHERE call_id=?`, id).Scan(&encoded)
	if errors.Is(err, sql.ErrNoRows) {
		return plugin.ToolCaller{}, false, nil
	}
	if err != nil {
		return plugin.ToolCaller{}, false, err
	}
	var value plugin.ToolCallerSnapshot
	if err := json.Unmarshal([]byte(encoded), &value, json.RejectUnknownMembers(true)); err != nil {
		return plugin.ToolCaller{}, false, err
	}
	caller, err := plugin.NewToolCaller(value)
	if err != nil {
		return plugin.ToolCaller{}, false, err
	}
	if value.Invocation.ID != id {
		return plugin.ToolCaller{}, false, plugin.ErrCommandConflict
	}
	return caller, true, nil
}
