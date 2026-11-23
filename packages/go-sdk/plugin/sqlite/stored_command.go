package sqlite

import (
	"context"

	framesqlite "github.com/Tangerg/frame/sqlite"
	framevalue "github.com/Tangerg/frame/value"
)

// storedCommand is the durable admission recorded under one command id; a nil
// reply means the effect has not been confirmed yet.
type storedCommand struct {
	method string
	input  string
	reply  []byte
}

type commandRowReader interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *framesqlite.Row
}

func readStoredCommand(ctx context.Context, rows commandRowReader, id string) (storedCommand, error) {
	var stored storedCommand
	err := rows.QueryRowContext(ctx, `SELECT method,business_input_json,reply FROM commands WHERE id=?`, id).
		Scan(&stored.method, &stored.input, &stored.reply)
	return stored, err
}

func (s storedCommand) admits(method string, input framevalue.JSON) bool {
	return s.method == method && s.input == input.String()
}
