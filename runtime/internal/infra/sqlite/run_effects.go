package sqlite

import (
	"github.com/Tangerg/flame/runtime/internal/domain/run"
)

type unresolvedEffectRow struct {
	ProcessID string `json:"processId"`
	EffectID  string `json:"effectId"`
	Cause     string `json:"cause"`
	Reason    string `json:"reason,omitempty"`
	Detail    string `json:"detail,omitempty"`
}

func encodeUnresolvedEffects(effects []run.UnresolvedEffect) (string, error) {
	rows := make([]unresolvedEffectRow, 0, len(effects))
	for _, e := range effects {
		rows = append(rows, unresolvedEffectRow{e.ProcessID(), e.EffectID(), e.Cause(), e.Reason(), e.Detail()})
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
		effect, err := run.NewUnresolvedEffect(row.ProcessID, row.EffectID, row.Cause, row.Reason, row.Detail)
		if err != nil {
			return nil, err
		}
		effects = append(effects, effect)
	}
	return effects, nil
}
