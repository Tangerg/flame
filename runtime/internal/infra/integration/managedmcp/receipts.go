package managedmcp

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	json "encoding/json/v2"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/idempotency"
	sqlitedb "github.com/Tangerg/flame/runtime/internal/infra/sqlite"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

const CommandsSchema = `CREATE TABLE IF NOT EXISTS commands (id TEXT PRIMARY KEY, method TEXT NOT NULL, input_digest TEXT NOT NULL, result TEXT NOT NULL CHECK(json_valid(result)))`

// Mutation and reply must commit together so redelivery cannot repeat a successful transition.
func Command[P, R any](ctx context.Context, db *sql.DB, method, id string, input P, handler func(context.Context, P) (R, error)) (R, error) {
	var zero R
	if len(id) == 0 || len(id) > 256 {
		return zero, errors.New("managed plugin: bounded command identity is required")
	}
	encoded, err := json.Marshal(input, json.Deterministic(true))
	if err != nil {
		return zero, err
	}
	sum := sha256.Sum256(encoded)
	digest := hex.EncodeToString(sum[:])
	var value R
	err = sqlitedb.RunInTx(ctx, db, func(ctx context.Context) error {
		var savedMethod, savedDigest, reply string
		err := sqlitedb.Connection(ctx, db).QueryRowContext(ctx, `SELECT method,input_digest,result FROM commands WHERE id=?`, id).Scan(&savedMethod, &savedDigest, &reply)
		if err == nil {
			if savedMethod != method || savedDigest != digest {
				return idempotency.ErrKeyConflict
			}
			return json.Unmarshal([]byte(reply), &value)
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		value, err = handler(ctx, input)
		if err != nil {
			return err
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			return err
		}
		_, err = sqlitedb.Connection(ctx, db).ExecContext(ctx, `INSERT INTO commands(id,method,input_digest,result) VALUES(?,?,?,?)`, id, method, digest, string(encoded))
		return err
	})
	if err != nil {
		return zero, err
	}
	return value, nil
}
func RegisterCommand[P, R any](server *sdk.Server, db *sql.DB, name string, classify func(error) string, handler func(context.Context, *sdk.ServerSession, P) (R, error)) error {
	return sdk.AddReceivingCustomMethod(server, name, func(ctx context.Context, session *sdk.ServerSession, params *Params[P]) (*Result[R], error) {
		if params == nil {
			return nil, fmt.Errorf("%s: parameters are required", name)
		}
		value, err := Command(ctx, db, name, params.CommandID, params.Input, func(ctx context.Context, input P) (R, error) { return handler(ctx, session, input) })
		return newResult(value, err, classify), nil
	})
}
