package main

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"os"
	"path/filepath"

	"github.com/Tangerg/flame/packages/go-sdk/plugin/backend"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func run(ctx context.Context, data string) (err error) {
	var specs []agentSpec
	if raw, configured := os.LookupEnv("A2A_AGENTS"); configured {
		if len(raw) > 1<<20 {
			return errors.New("a2a: A2A_AGENTS exceeds the configuration limit")
		}
		if err := json.Unmarshal([]byte(raw), &specs, json.RejectUnknownMembers(true)); err != nil || specs == nil {
			return errors.New("a2a: A2A_AGENTS must be a valid agent configuration array")
		}
	}
	connector, err := openConnector(ctx, filepath.Join(data, "a2a.db"), specs)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, connector.db.Close()) }()
	server, err := newServer(connector)
	if err != nil {
		return err
	}
	return server.Run(ctx, &sdk.StdioTransport{})
}

func main() { backend.Main("a2a", run) }
