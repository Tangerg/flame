package persistence

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/qimu"
	"github.com/Tangerg/flame/qimu/sqlite"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

type referenceWriteKey struct{}

type referenceWrite struct {
	checkpoint runs.ExecutorCheckpoint
}

// PluginReferenceTransactor is the stable checkpoint commit port bound by Open.
// It joins the original product write-set to Qimu's one
// admission and stable storage settlement. Product receipts remain in fn's
// original tables; the framework receipt identifies only this storage attempt.
type PluginReferenceTransactor func(ctx context.Context, host *qimu.Host, identity string, checkpoint runs.ExecutorCheckpoint, fn func(context.Context) error) error

func (b *Bundle) commitPluginReferences(ctx context.Context, host *qimu.Host, identity string, checkpoint runs.ExecutorCheckpoint, fn func(context.Context) error) error {
	if host == nil || host.Admission() != b.Admission || identity == "" || fn == nil {
		return errors.New("persistence: original host, checkpoint identity and write-set are required")
	}
	if err := checkpoint.Validate(); err != nil {
		return err
	}
	checkpoint = checkpoint.Clone()
	encoded, err := encodeCheckpointIdentity(checkpoint)
	if err != nil {
		return err
	}
	request, err := json.Marshal(struct {
		Kind       string      `json:"kind"`
		Identity   string      `json:"identity"`
		Checkpoint qimu.Digest `json:"checkpoint"`
	}{"execution.checkpoint", identity, qimu.Hash(encoded)})
	if err != nil {
		return err
	}
	return b.Admission.Admit(ctx, func(admitted context.Context) error {
		return b.commitPluginWrite(admitted, request, func(txCtx context.Context) error {
			previous, err := b.ExecutorCheckpoints.LoadCheckpoint(txCtx, checkpoint.RootMemberID)
			if err != nil && !errors.Is(err, runs.ErrExecutorCheckpointNotFound) {
				return err
			}
			for _, activation := range checkpoint.Bindings {
				binding, err := plugins.DecodeActivation(activation)
				if err != nil {
					return err
				}
				preserved := slices.ContainsFunc(previous.Bindings, activation.Equal)
				if !preserved {
					if err := host.ValidateBindingUnder(txCtx, binding); err != nil {
						return err
					}
				}
			}
			for _, resource := range checkpoint.Resources {
				if slices.Contains(previous.Resources, resource) {
					continue
				}
				installation, exists := host.Installation(qimu.InstallationID(resource.InstallationID.String()))
				if !exists || !installation.Enabled || installation.Release.String() != resource.Digest.String() {
					return fmt.Errorf("%w: retained content source changed", qimu.ErrStaleBinding)
				}
			}
			return fn(context.WithValue(txCtx, referenceWriteKey{}, referenceWrite{checkpoint: checkpoint}))
		})
	})
}

// The technical request identifies the original storage write-set. It encodes
// domain identities explicitly and retains no independently mutable graph.
func encodeCheckpointIdentity(checkpoint runs.ExecutorCheckpoint) ([]byte, error) {
	type contentSource struct {
		Installation string `json:"installation"`
		Release      string `json:"release"`
	}
	bindings := make([]string, len(checkpoint.Bindings))
	for index, activation := range checkpoint.Bindings {
		bindings[index] = activation.Binding.String()
	}
	resources := make([]contentSource, len(checkpoint.Resources))
	for index, dependency := range checkpoint.Resources {
		resources[index] = contentSource{dependency.InstallationID.String(), dependency.Digest.String()}
	}
	return json.Marshal(struct {
		Root        string                    `json:"root"`
		Session     string                    `json:"session"`
		Build       string                    `json:"build"`
		Payload     []byte                    `json:"payload"`
		ToolResults []transcript.ToolResultID `json:"toolResults"`
		Bindings    []string                  `json:"bindings"`
		Resources   []contentSource           `json:"resources"`
	}{checkpoint.RootMemberID, checkpoint.SessionID, checkpoint.BuildID, checkpoint.Payload, checkpoint.ToolResultIDs, bindings, resources})
}

func (b *Bundle) commitPluginWrite(ctx context.Context, request []byte, fn func(context.Context) error) error {
	if err := b.Admission.CheckAdmitted(ctx); err != nil {
		return err
	}
	if err := b.ValidateDirectory(); err != nil {
		return err
	}
	id, err := b.PluginUses.PrepareUnder(ctx, request)
	if err != nil {
		return err
	}
	settlement, err := b.PluginUses.CommitUnder(ctx, id, request, func(admitted context.Context, executor sqlite.Executor) ([]byte, error) {
		if err := b.Qimu.WithTransaction(admitted, executor, fn); err != nil {
			return nil, err
		}
		return request, nil
	}, nil)
	if settlement.State == sqlite.Indeterminate || !settlement.Synchronized {
		b.Admission.RequireRecovery(errors.Join(sqlite.ErrUnsettled, err))
		return errors.Join(qimu.ErrRecoveryRequired, err)
	}
	return errors.Join(err, b.PluginUses.CompactSettledUnder(ctx))
}

// RecoverPluginBindings settles original storage attempts only. All product
// facts were committed in their transaction and readers query those original
// tables; there is no independently advancing retention registry to rebuild.
func (b *Bundle) RecoverPluginBindings(ctx context.Context) error {
	if err := b.PluginUses.RecoverPrepared(ctx); err != nil {
		return err
	}
	pending, err := b.PluginUses.Pending(ctx)
	if err != nil {
		return err
	}
	for _, settlement := range pending {
		if settlement.State != sqlite.Committed && settlement.State != sqlite.NotCommitted {
			return sqlite.ErrUnsettled
		}
		if err := b.PluginUses.Confirm(ctx, settlement.ID); err != nil {
			return err
		}
	}
	return b.PluginUses.CompactSettled(ctx)
}

// PendingPluginBindings reads the original business owners in one snapshot.
// Settled commands and completed execution owners no longer pin a setup graph;
// no copied registry can outlive their decision.
func (b *Bundle) PendingPluginBindings(ctx context.Context) ([]plugin.Activation, error) {
	var result []plugin.Activation
	err := sqlite.RunReadSnapshot(ctx, b.db, func(ctx context.Context) error {
		commands, err := newPluginCommandStore(b.db, nil).pendingBindings(ctx)
		if err != nil {
			return err
		}
		checkpoints, _, err := b.ExecutorCheckpoints.pendingReferences(ctx)
		if err != nil {
			return err
		}
		for _, binding := range append(commands, checkpoints...) {
			if !slices.ContainsFunc(result, binding.Equal) {
				result = append(result, binding.Clone())
			}
		}
		return nil
	})
	return result, err
}

func validateReferenceWrite(ctx context.Context, checkpoint runs.ExecutorCheckpoint) error {
	write, exists := ctx.Value(referenceWriteKey{}).(referenceWrite)
	if !exists || !write.checkpoint.Equal(checkpoint) {
		return errors.New("persistence: checkpoint differs from its original admitted write-set")
	}
	return nil
}

func checkpointDependencies(bindings []plugin.Activation, resources []plugin.Dependency) ([]plugin.Dependency, error) {
	result := slices.Clone(resources)
	for _, activation := range bindings {
		binding, err := plugins.DecodeActivation(activation)
		if err != nil {
			return nil, err
		}
		original, err := plugins.ProjectActivation(binding)
		if err != nil {
			return nil, err
		}
		result = append(result, original.Dependencies...)
	}
	return plugin.CompactDependencies(result), nil
}

func cloneActivations(values []plugin.Activation) []plugin.Activation {
	result := slices.Clone(values)
	for index := range result {
		result[index] = result[index].Clone()
	}
	return result
}
