package main

import (
	"context"
	"database/sql"
	json "encoding/json/v2"
	"errors"
	"maps"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	"github.com/Tangerg/flame/plugins/a2a/internal/wire"
	"github.com/Tangerg/flame/qimu/jsonvalue"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
	"github.com/Tangerg/flame/qimu/sqlite"
	"github.com/a2aproject/a2a-go/v2/a2a"
)

type connector struct {
	db     *sql.DB
	agents map[string]agent
}

// A binding records where the original send occurred. Remote status is never
// persisted here: it remains the remote agent's fact.
type taskBinding struct {
	Agent     string             `json:"agent"`
	Authority identity.Digest    `json:"authority"`
	Endpoint  a2a.AgentInterface `json:"endpoint"`
	TaskID    a2a.TaskID         `json:"taskId"`
	ContextID string             `json:"contextId"`
}

func openConnector(ctx context.Context, path string, specs []agentSpec) (*connector, error) {
	agents, err := configureAgents(specs)
	if err != nil {
		return nil, err
	}
	db, err := sqlite.OpenSchema(ctx, path, []string{
		pluginstorage.CommandsSchema,
		`CREATE TABLE IF NOT EXISTS bindings (request_id TEXT PRIMARY KEY REFERENCES commands(id), binding TEXT NOT NULL CHECK(json_valid(binding)))`,
	})
	if err != nil {
		return nil, err
	}
	return &connector{db: db, agents: agents}, nil
}

func (c *connector) list(context.Context, struct{}) (wire.Agents, error) {
	result := wire.Agents{Agents: make([]wire.Agent, 0, len(c.agents))}
	for _, name := range slices.Sorted(maps.Keys(c.agents)) {
		result.Agents = append(result.Agents, c.agents[name].info)
	}
	return result, nil
}

func (c *connector) send(ctx context.Context, method, id string, input jsonvalue.Value) (_ wire.Result, err error) {
	var request wire.SendRequest
	var selected agent
	prepared, reply, err := pluginstorage.AdmitCommand(ctx, c.db, method, id, input, func(context.Context) error {
		if err := json.Unmarshal(input.JSON(), &request, json.RejectUnknownMembers(true)); err != nil {
			return invalidRequest()
		}
		if !utf8.ValidString(request.Message) || len(request.Message) > 64<<10 || strings.TrimSpace(request.Message) == "" {
			return invalidRequest()
		}
		var found bool
		selected, found = c.agents[request.Agent]
		if !found {
			return &portable.Refusal{Code: "agent_unconfigured", Message: "a2a: agent is not configured"}
		}
		return nil
	})
	if err != nil {
		return wire.Result{}, err
	}
	if prepared == nil {
		return qimumcp.DecodeReply[wire.Result](reply)
	}
	if !prepared.NewlyAdmitted() {
		return wire.Result{}, pluginstorage.ErrCommandUnconfirmed
	}
	client, endpoint, err := selected.discover(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return wire.Result{}, errors.Join(err, ctx.Err(), context.Cause(ctx))
		}
		return complete(ctx, prepared, wire.Result{}, &portable.Refusal{Code: "discovery_failed", Message: "a2a: agent discovery or endpoint admission failed"})
	}
	defer func() { err = errors.Join(err, client.Destroy()) }()
	message := a2a.NewMessage(a2a.MessageRoleUser, a2a.NewTextPart(request.Message))
	remote, err := client.SendMessage(ctx, &a2a.SendMessageRequest{Message: message, Config: &a2a.SendMessageConfig{ReturnImmediately: true}})
	if err != nil {
		return wire.Result{}, &remoteError{operation: "send", cause: err}
	}
	result, err := project(id, request.Agent, remote)
	if err != nil {
		return wire.Result{}, err
	}
	return result, sqlite.RunInTx(ctx, c.db, func(ctx context.Context) error {
		if result.TaskID != "" {
			binding := taskBinding{Agent: request.Agent, Authority: selected.authority, Endpoint: endpoint, TaskID: a2a.TaskID(result.TaskID), ContextID: result.ContextID}
			data, err := json.Marshal(binding)
			if err != nil {
				return err
			}
			if _, err := sqlite.Connection(ctx, c.db).ExecContext(ctx, `INSERT INTO bindings(request_id,binding) VALUES(?,?)`, id, string(data)); err != nil {
				return err
			}
		}
		_, err := complete(ctx, prepared, result, nil)
		return err
	})
}

func (c *connector) status(ctx context.Context, request wire.TaskRequest) (_ wire.Result, err error) {
	binding, selected, err := c.binding(ctx, request.RequestID)
	if err != nil {
		return wire.Result{}, err
	}
	client, err := selected.connect(ctx, binding.Endpoint)
	if err != nil {
		return wire.Result{}, err
	}
	defer func() { err = errors.Join(err, client.Destroy()) }()
	task, err := client.GetTask(ctx, &a2a.GetTaskRequest{ID: binding.TaskID, HistoryLength: new(0)})
	if err != nil {
		return wire.Result{}, &remoteError{operation: "status", cause: err}
	}
	return binding.project(request.RequestID, task)
}

func (c *connector) cancel(ctx context.Context, method, id string, input jsonvalue.Value) (_ wire.Result, err error) {
	var request wire.TaskRequest
	var binding taskBinding
	var selected agent
	prepared, reply, err := pluginstorage.AdmitCommand(ctx, c.db, method, id, input, func(ctx context.Context) error {
		if err := json.Unmarshal(input.JSON(), &request, json.RejectUnknownMembers(true)); err != nil {
			return invalidRequest()
		}
		var err error
		binding, selected, err = c.binding(ctx, request.RequestID)
		return err
	})
	if err != nil {
		return wire.Result{}, err
	}
	if prepared == nil {
		return qimumcp.DecodeReply[wire.Result](reply)
	}
	if !prepared.NewlyAdmitted() {
		return wire.Result{}, pluginstorage.ErrCommandUnconfirmed
	}
	client, err := selected.connect(ctx, binding.Endpoint)
	if err != nil {
		return complete(ctx, prepared, wire.Result{}, &portable.Refusal{Code: "endpoint_rejected", Message: "a2a: original endpoint cannot be admitted"})
	}
	defer func() { err = errors.Join(err, client.Destroy()) }()
	task, err := client.CancelTask(ctx, &a2a.CancelTaskRequest{ID: binding.TaskID})
	if err != nil {
		return wire.Result{}, &remoteError{operation: "cancel", cause: err}
	}
	result, err := binding.project(request.RequestID, task)
	if err != nil {
		return wire.Result{}, err
	}
	return complete(ctx, prepared, result, nil)
}

func (c *connector) binding(ctx context.Context, id string) (taskBinding, agent, error) {
	if id == "" || len(id) > qimumcp.MaxCommandIDBytes || !utf8.ValidString(id) {
		return taskBinding{}, agent{}, invalidRequest()
	}
	var raw string
	err := sqlite.Connection(ctx, c.db).QueryRowContext(ctx, `SELECT binding FROM bindings WHERE request_id=?`, id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return taskBinding{}, agent{}, &portable.Refusal{Code: "task_unavailable", Message: "a2a: original request has no confirmed remote task binding"}
	}
	if err != nil {
		return taskBinding{}, agent{}, err
	}
	var binding taskBinding
	if err := json.Unmarshal([]byte(raw), &binding, json.RejectUnknownMembers(true)); err != nil {
		return taskBinding{}, agent{}, err
	}
	if binding.TaskID == "" || binding.ContextID == "" || binding.Authority.Validate() != nil {
		return taskBinding{}, agent{}, errors.New("a2a: invalid stored task binding")
	}
	selected, found := c.agents[binding.Agent]
	if !found || selected.authority != binding.Authority {
		return taskBinding{}, agent{}, &portable.Refusal{Code: "agent_changed", Message: "a2a: original agent authority is no longer configured"}
	}
	return binding, selected, nil
}

func (b taskBinding) project(id string, task *a2a.Task) (wire.Result, error) {
	if task == nil || task.ID != b.TaskID || task.ContextID != b.ContextID {
		return wire.Result{}, errors.New("a2a: response differs from the original task identity")
	}
	return project(id, b.Agent, task)
}

func project(id, agent string, remote a2a.SendMessageResult) (wire.Result, error) {
	result := wire.Result{RequestID: id, Agent: agent}
	switch value := remote.(type) {
	case *a2a.Task:
		if value == nil || value.ID == "" || value.ContextID == "" {
			return wire.Result{}, errors.New("a2a: response has no remote task identity")
		}
		result.Kind, result.TaskID, result.ContextID, result.State = "task", string(value.ID), value.ContextID, value.Status.State.String()
	case *a2a.Message:
		if value == nil || value.ID == "" {
			return wire.Result{}, errors.New("a2a: response has no remote message identity")
		}
		result.Kind, result.TaskID, result.ContextID = "message", string(value.TaskID), value.ContextID
		if result.TaskID != "" && result.ContextID == "" {
			return wire.Result{}, errors.New("a2a: message has no task context identity")
		}
	default:
		return wire.Result{}, errors.New("a2a: response is neither a task nor a message")
	}
	data, err := json.Marshal(remote)
	if err != nil {
		return wire.Result{}, err
	}
	if len(data) > maxResponseBytes {
		return wire.Result{}, errResponseLimit
	}
	result.ValueJSON = string(data)
	return result, nil
}

func complete(ctx context.Context, prepared *pluginstorage.PreparedCommand, result wire.Result, operationErr error) (wire.Result, error) {
	encoded, err := qimumcp.EncodeReply(result, operationErr, nil)
	if err != nil {
		return wire.Result{}, err
	}
	if err := prepared.Complete(ctx, encoded.JSON()); err != nil {
		return wire.Result{}, err
	}
	return qimumcp.DecodeReply[wire.Result](encoded.JSON())
}

func invalidRequest() error {
	return &portable.Refusal{Code: "invalid_request", Message: "a2a: invalid request"}
}
