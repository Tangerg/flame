package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	json "encoding/json/v2"
	"errors"
	"io"
	"strings"

	runtimeclient "github.com/Tangerg/flame/packages/go-sdk/client"
	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/identity"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	storage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/plugins/knowledgebase/internal/knowledge"
	"github.com/Tangerg/flame/plugins/knowledgebase/internal/wire"
	"github.com/Tangerg/flame/plugins/researchartifact"
	framecall "github.com/Tangerg/frame/call"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

const methodSchema = `CREATE TABLE IF NOT EXISTS research_methods (
	project TEXT NOT NULL, version TEXT NOT NULL, method TEXT NOT NULL CHECK(json_valid(method)), PRIMARY KEY(project, version))`

const workSchema = `CREATE TABLE IF NOT EXISTS research_work (
	project TEXT NOT NULL, id TEXT NOT NULL, revision INTEGER NOT NULL, title TEXT NOT NULL,
	method_version TEXT NOT NULL, work TEXT NOT NULL CHECK(json_valid(work)),
	FOREIGN KEY(project, method_version) REFERENCES research_methods(project, version),
	PRIMARY KEY (project, id, revision))`

var errWorkInvalid = errors.New("research work requires a method, current claim revisions, and an exact Project deliverable revision")

type workVersion struct {
	Claims   []wire.ClaimRef  `json:"claims"`
	Artifact wire.ArtifactRef `json:"artifact"`
	SHA256   string           `json:"sha256"`
}

type artifactBindings map[string]pluginmcp.SetupBinding

func researchBindings(setup pluginmcp.Setup) artifactBindings {
	bindings := artifactBindings{}
	for _, contribution := range setup.Contributions() {
		if contribution.ID != "knowledgebase-operations" {
			continue
		}
		for _, dependency := range contribution.Requires {
			if dependency.ID == "research-artifacts" {
				for _, binding := range dependency.Bindings {
					bindings[binding.InstallationID] = binding
				}
			}
		}
	}
	return bindings
}

func (l *library) readArtifact(ctx context.Context, project string, selected wire.ArtifactRef) (result researchartifact.Evidence, resultErr error) {
	l.bindingsMu.RLock()
	binding, present := l.artifacts[selected.InstallationID]
	l.bindingsMu.RUnlock()
	if !present {
		return result, nil
	}
	published, method, err := researchartifact.Contract()
	if err != nil {
		return result, err
	}
	ref, err := pluginmcp.CallbackFromContext(ctx)
	if err != nil {
		return result, err
	}
	client, err := runtimeclient.ConnectCallback(ctx, ref)
	if err != nil {
		return result, err
	}
	defer func() { resultErr = errors.Join(resultErr, client.Close()) }()
	read, err := runtimeclient.BindPluginMethod(client, runtimeclient.PluginBinding{
		InstallationID: binding.InstallationID, Digest: binding.Digest,
		Contribution: binding.Contribution, Contract: published,
		Target: framecall.Target{Scope: extension.ScopeInstallation, Reference: binding.InstallationID},
	}, method)
	if err != nil {
		return result, err
	}
	return read.Call(ctx, researchartifact.Request{
		Project: project, ID: selected.ID, Revision: selected.Revision, Object: selected.Object,
	}, protocol.PluginCallOptions{})
}

func (l *library) acceptWork(ctx context.Context, command pluginmcp.Command[wire.WorkAccept]) (wire.WorkReceipt, error) {
	var empty wire.WorkReceipt
	call, err := pluginmcp.InvocationFromContext(ctx)
	if err != nil {
		return empty, err
	}
	if call.Origin != extension.Human {
		return empty, errHumanClaim
	}
	request := command.Input
	if err := checkClaimProject(request.Project); err != nil {
		return empty, err
	}
	if _, err := identity.ParseInstallation(request.Artifact.InstallationID); err != nil ||
		strings.TrimSpace(request.Title) == "" || strings.TrimSpace(request.Method.Name) == "" ||
		strings.TrimSpace(request.Method.Instructions) == "" {
		return empty, errWorkInvalid
	}
	admitted, err := command.RequestInput()
	if err != nil {
		return empty, err
	}
	prepared, receipt, err := storage.AdmitCommand(ctx, l.store.db, wire.AcceptWork, string(command.ID), admitted, nil)
	if err != nil {
		return empty, err
	}
	if prepared == nil {
		return pluginmcp.DecodeReply[wire.WorkReceipt](receipt)
	}
	evidence, artifactErr := l.readArtifact(ctx, request.Project, request.Artifact)
	return storage.CompleteExternalCommand(ctx, prepared, classify, func(tx context.Context) (storage.CommandPlan[wire.WorkReceipt], error) {
		if artifactErr != nil {
			return storage.CommandPlan[wire.WorkReceipt]{}, artifactErr
		}
		if !evidence.Available {
			return storage.CommandPlan[wire.WorkReceipt]{}, errWorkInvalid
		}
		if evidence.Head != request.Artifact.Revision || !evidence.ObjectAtHead {
			return storage.CommandPlan[wire.WorkReceipt]{}, knowledge.ErrRevisionConflict
		}
		return l.store.prepareWork(tx, request, evidence.AcceptedSHA256)
	})
}

func (s *store) prepareWork(ctx context.Context, input wire.WorkAccept, documentSHA string) (storage.CommandPlan[wire.WorkReceipt], error) {
	var empty storage.CommandPlan[wire.WorkReceipt]
	db := s.db.Executor(ctx)
	var head, count int64
	if err := db.QueryRowContext(ctx, `SELECT COALESCE(MAX(revision),0) FROM research_work WHERE project=? AND id=?`, input.Project, input.ID).Scan(&head); err != nil {
		return empty, err
	}
	if head != input.ExpectedRevision {
		return empty, knowledge.ErrRevisionConflict
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM research_work WHERE project=?`, input.Project).Scan(&count); err != nil {
		return empty, err
	}
	if count >= 256 {
		return empty, knowledge.ErrCapacity
	}
	seen := map[string]bool{}
	for _, ref := range input.Claims {
		if seen[ref.ID] {
			return empty, errWorkInvalid
		}
		seen[ref.ID] = true
		claim, err := s.readClaim(ctx, wire.ClaimReadInput{Project: input.Project, ID: ref.ID})
		if err != nil {
			return empty, err
		}
		if claim.State != wire.ClaimCurrent || claim.Revision != ref.Revision {
			return empty, knowledge.ErrRevisionConflict
		}
	}
	method, err := json.Marshal(input.Method, json.Deterministic(true))
	if err != nil {
		return empty, err
	}
	digest := sha256.Sum256(method)
	version := hex.EncodeToString(digest[:])
	value, err := json.Marshal(workVersion{Claims: input.Claims, Artifact: input.Artifact, SHA256: documentSHA})
	if err != nil {
		return empty, err
	}
	return storage.NewCommandPlan(wire.WorkReceipt{ID: input.ID, Revision: head + 1, MethodVersion: version}, func(tx context.Context) error {
		db := s.db.Executor(tx)
		if _, err := db.ExecContext(tx, `INSERT INTO research_methods(project,version,method) VALUES(?,?,?)
			ON CONFLICT(project,version) DO NOTHING`, input.Project, version, string(method)); err != nil {
			return err
		}
		_, err := db.ExecContext(tx, `INSERT INTO research_work(project,id,revision,title,method_version,work)
			VALUES(?,?,?,?,?,?)`, input.Project, input.ID, head+1, input.Title, version, string(value))
		return err
	})
}

func (l *library) readWork(ctx context.Context, input wire.WorkRead) (wire.WorkView, error) {
	view := wire.WorkView{State: "current", Claims: []wire.ClaimRead{}}
	if err := checkClaimProject(input.Project); err != nil {
		return view, err
	}
	view.ID = input.ID
	var raw, method string
	var saved workVersion
	var hidden bool
	err := l.store.db.RunReadSnapshot(ctx, func(tx context.Context) error {
		err := l.store.db.Executor(tx).QueryRowContext(tx, `SELECT w.revision,w.title,w.method_version,w.work,m.method
			FROM research_work w JOIN research_methods m USING(project)
			WHERE w.method_version=m.version AND w.project=? AND w.id=? AND (?=0 OR w.revision=?)
			ORDER BY w.revision DESC LIMIT 1`, input.Project, input.ID, input.Revision, input.Revision).
			Scan(&view.Revision, &view.Title, &view.MethodVersion, &raw, &method)
		if errors.Is(err, sql.ErrNoRows) {
			return knowledge.ErrNotFound
		}
		if err != nil {
			return err
		}
		if err := json.Unmarshal([]byte(raw), &saved); err != nil {
			return err
		}
		for _, ref := range saved.Claims {
			exact, err := l.store.readClaim(tx, wire.ClaimReadInput{Project: input.Project, ID: ref.ID, Revision: ref.Revision})
			if err != nil {
				return err
			}
			var head int64
			if err := l.store.db.Executor(tx).QueryRowContext(tx, `SELECT MAX(revision) FROM research_claims WHERE project=? AND id=?`, input.Project, ref.ID).Scan(&head); err != nil {
				return err
			}
			if exact.State != wire.ClaimCurrent || head != ref.Revision {
				view.State = "recheck"
			}
			hidden = hidden || exact.State == wire.ClaimUnavailable
			view.Claims = append(view.Claims, exact)
		}
		return nil
	})
	if err != nil {
		return view, err
	}
	if hidden {
		view.Title = ""
		return view, nil
	}
	view.Method = new(wire.ResearchMethod)
	if err := json.Unmarshal([]byte(method), view.Method); err != nil {
		return view, err
	}
	evidence, err := l.readArtifact(ctx, input.Project, saved.Artifact)
	if err != nil {
		return view, err
	}
	if !evidence.Available || evidence.AcceptedSHA256 != saved.SHA256 {
		view.State = "recheck"
		return view, nil
	}
	if evidence.Head != saved.Artifact.Revision || !evidence.ObjectAtHead {
		view.State = "recheck"
	}
	view.Artifact, view.Evidence = &saved.Artifact, &evidence
	return view, nil
}

func (s *store) listWork(ctx context.Context, input wire.ListInput) ([]wire.WorkReceipt, error) {
	result := []wire.WorkReceipt{}
	if err := checkClaimProject(input.Project); err != nil {
		return result, err
	}
	err := s.db.RunReadSnapshot(ctx, func(tx context.Context) error {
		rows, err := s.db.Executor(tx).QueryContext(tx, `SELECT w.id,w.revision,w.method_version FROM research_work w
			WHERE w.project=? AND w.revision=(SELECT MAX(h.revision) FROM research_work h WHERE h.project=w.project AND h.id=w.id)
			ORDER BY w.rowid DESC`, input.Project)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var work wire.WorkReceipt
			if err := rows.Scan(&work.ID, &work.Revision, &work.MethodVersion); err != nil {
				return err
			}
			result = append(result, work)
		}
		return rows.Err()
	})
	return result, err
}

func (l *library) registerWork(backend *pluginmcp.Backend, methods workMethods) error {
	return errors.Join(
		backend.OnSetup(func(_ context.Context, setup pluginmcp.Setup) (io.Closer, error) {
			l.bindingsMu.Lock()
			l.artifacts = researchBindings(setup)
			l.bindingsMu.Unlock()
			return nil, nil
		}),
		methods.accept.RegisterServer(backend, classify, func(ctx context.Context, _ *sdk.ServerSession, in pluginmcp.Command[wire.WorkAccept]) (wire.WorkReceipt, error) {
			return l.acceptWork(ctx, in)
		}),
		methods.read.RegisterServer(backend, classify, func(ctx context.Context, _ *sdk.ServerSession, in pluginmcp.Command[wire.WorkRead]) (wire.WorkView, error) {
			return l.readWork(ctx, in.Input)
		}),
		methods.list.RegisterServer(backend, classify, func(ctx context.Context, _ *sdk.ServerSession, in pluginmcp.Command[wire.ListInput]) ([]wire.WorkReceipt, error) {
			return l.store.listWork(ctx, in.Input)
		}),
	)
}

type workMethods struct {
	accept *pluginmcp.Method[wire.WorkAccept, wire.WorkReceipt]
	read   *pluginmcp.Method[wire.WorkRead, wire.WorkView]
	list   *pluginmcp.Method[wire.ListInput, []wire.WorkReceipt]
}
