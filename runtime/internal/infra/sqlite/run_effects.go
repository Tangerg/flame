package sqlite

import (
	json "encoding/json/v2"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/scope/core/chat"
)

type unresolvedEffectRow struct {
	ProcessID string `json:"processId"`
	EffectID  string `json:"effectId"`
	Cause     string `json:"cause"`
	Reason    string `json:"reason,omitempty"`
	Detail    string `json:"detail,omitempty"`
	Output    string `json:"output,omitempty"`
}

func encodeUnresolvedEffects(effects []run.UnresolvedEffect) (string, error) {
	rows := make([]unresolvedEffectRow, 0, len(effects))
	for _, e := range effects {
		rows = append(rows, unresolvedEffectRow{ProcessID: e.ProcessID(), EffectID: e.EffectID(), Cause: e.Cause(), Reason: e.Reason(), Detail: e.Detail(), Output: e.Output()})
	}
	encoded, err := encodeStoredJSON(rows)
	return string(encoded), err
}
func decodeUnresolvedEffects(encoded string) ([]run.UnresolvedEffect, error) {
	var rows []unresolvedEffectRow
	if err := decodeStoredJSON([]byte(encoded), &rows); err != nil {
		return nil, err
	}
	var effects []run.UnresolvedEffect
	for _, row := range rows {
		var output *chat.ToolOutput
		if len(row.Output) > 0 {
			output = new(chat.ToolOutput)
			if err := json.Unmarshal([]byte(row.Output), output); err != nil {
				return nil, err
			}
		}
		effect, err := run.NewUnresolvedEffect(run.UnresolvedEffectConfig{
			ProcessID: row.ProcessID, EffectID: row.EffectID, Cause: row.Cause, Reason: row.Reason, Detail: row.Detail, Output: output,
		})
		if err != nil {
			return nil, err
		}
		effects = append(effects, effect)
	}
	return effects, nil
}
