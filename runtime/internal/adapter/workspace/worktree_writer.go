package workspace

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	work "github.com/Tangerg/flame/runtime/internal/application/workspace"
	"github.com/Tangerg/flame/runtime/internal/infra/git"
	"github.com/google/uuid"
)

const (
	taskWriterExecutable = "flame-worktree-writer"
	taskWriteDirectory   = "worktree-writes"
	taskWriteReceipt     = "receipt.json"
)

type taskWriter struct {
	directory  string
	executable string
}

func openTaskWriter(ctx context.Context, dataDirectory string) (*taskWriter, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	directory := filepath.Join(dataDirectory, taskWriteDirectory)
	if err := os.MkdirAll(directory, filesystem.PrivateDirectoryMode); err != nil {
		return nil, err
	}
	// A replacement Runtime must join the predecessor's admitted writes before
	// its in-memory workspace gate can admit Sessions or new writes.
	lease, err := filesystem.AcquireDirectoryLease(ctx, directory)
	if err != nil {
		return nil, err
	}
	if err := lease.Release(); err != nil {
		return nil, err
	}
	return &taskWriter{directory: directory, executable: filepath.Join(filepath.Dir(executable), taskWriterExecutable)}, nil
}

type taskWriteKind string

const (
	writeHandoff taskWriteKind = "handoff"
	writeRelease taskWriteKind = "release"
)

type taskWriteRequest struct {
	ID      uuid.UUID      `json:"id"`
	Kind    taskWriteKind  `json:"kind"`
	Plan    jsontext.Value `json:"plan"`
	Success []byte         `json:"success"`
}

type handoffWrite struct {
	Destination string `json:"destination"`
	Patch       string `json:"patch"`
}

func (w handoffWrite) run(ctx context.Context) error {
	return applyWorktreePatch(ctx, w.Destination, w.Patch)
}

type releaseWrite struct {
	Manifest     string           `json:"manifest"`
	Registration *checkoutRemoval `json:"registration,omitzero"`
}

type checkoutRemoval struct {
	Repository string `json:"repository"`
	Checkout   string `json:"checkout"`
}

func (w releaseWrite) run(ctx context.Context) error {
	root, err := os.OpenRoot(filepath.Dir(w.Manifest))
	if err != nil {
		return err
	}
	defer root.Close()
	if w.Registration != nil {
		if err := git.RemoveWorktree(ctx, w.Registration.Repository, w.Registration.Checkout); err != nil {
			return errors.Join(work.ErrTaskWorktreeChanged, err)
		}
	}
	return root.Remove(filepath.Base(w.Manifest))
}

func (w *taskWriter) prepare(kind taskWriteKind, action any) (operation.ExternalExecution, error) {
	plan, err := json.Marshal(action)
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context, id uuid.UUID, success []byte) error {
		return w.start(ctx, taskWriteRequest{ID: id, Kind: kind, Plan: plan, Success: success})
	}, nil
}

func (w *taskWriter) start(ctx context.Context, request taskWriteRequest) (err error) {
	message, err := json.Marshal(request)
	if err != nil {
		return err
	}
	command := exec.CommandContext(ctx, w.executable, w.directory)
	lease, err := retainTaskWriterDirectory(command, w.directory)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, lease.Close()) }()
	command.Stdin = bytes.NewReader(message)
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("workspace: task writer: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func (w *taskWriter) ReadReceipt(id uuid.UUID) ([]byte, error) {
	root, err := os.OpenRoot(w.directory)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	payload, err := root.ReadFile(filepath.Join(id.String(), taskWriteReceipt))
	if !errors.Is(err, os.ErrNotExist) {
		return payload, err
	}
	if _, err := root.Stat(id.String()); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	return nil, operation.ErrInProgress
}

func (w *taskWriter) RetireReceipt(id uuid.UUID) error {
	root, err := os.OpenRoot(w.directory)
	if err != nil {
		return err
	}
	defer root.Close()
	return root.RemoveAll(id.String())
}

func RunTaskWriter(ctx context.Context, directory string, input io.Reader, refuse func(error) ([]byte, error)) error {
	lease := inheritedTaskWriterDirectory()
	if lease == nil {
		return errors.New("workspace: the task writer requires its original directory lease")
	}
	defer lease.Close()
	root, err := os.OpenRoot(directory)
	if err != nil {
		return err
	}
	defer root.Close()
	original, err := lease.Stat()
	if err != nil {
		return err
	}
	current, err := root.Stat(".")
	if err != nil {
		return err
	}
	if !os.SameFile(original, current) {
		return errors.New("workspace: the original task writer directory changed")
	}
	var request taskWriteRequest
	if err := json.UnmarshalRead(input, &request, json.RejectUnknownMembers(true)); err != nil {
		return err
	}
	run, err := request.execution()
	if err != nil {
		return err
	}
	if err := root.Mkdir(request.ID.String(), filesystem.PrivateDirectoryMode); err != nil {
		return err
	}
	if err := lease.Sync(); err != nil {
		return err
	}
	accepted, err := root.OpenRoot(request.ID.String())
	if err != nil {
		return err
	}
	defer accepted.Close()
	payload := request.Success
	if cause := run(ctx); cause != nil {
		payload, err = refuse(cause)
		if err != nil {
			return err
		}
	}
	return publishTaskFile(accepted, taskWriteReceipt, payload)
}

func (request taskWriteRequest) execution() (func(context.Context) error, error) {
	if request.ID == uuid.Nil || len(request.Success) == 0 {
		return nil, errors.New("workspace: a task write requires its receipt identity and response")
	}
	switch request.Kind {
	case writeHandoff:
		var write handoffWrite
		if err := json.Unmarshal(request.Plan, &write, json.RejectUnknownMembers(true)); err != nil {
			return nil, err
		}
		if !filepath.IsAbs(write.Destination) {
			return nil, errors.New("workspace: handoff destination must be absolute")
		}
		return write.run, nil
	case writeRelease:
		var write releaseWrite
		if err := json.Unmarshal(request.Plan, &write, json.RejectUnknownMembers(true)); err != nil {
			return nil, err
		}
		if !filepath.IsAbs(write.Manifest) || write.Registration != nil &&
			(!filepath.IsAbs(write.Registration.Repository) || !filepath.IsAbs(write.Registration.Checkout)) {
			return nil, errors.New("workspace: release requires absolute paths")
		}
		return write.run, nil
	default:
		return nil, errors.New("workspace: unknown task write")
	}
}
