package host

import (
	"context"

	"github.com/Tangerg/flame/qimu"
	"github.com/Tangerg/flame/qimu/control"
	"github.com/Tangerg/flame/qimu/sqlite"
)

// AcquireBinding is the trusted consumer port for one exact declared export.
// It activates the supplied graph without selection. Without a parent, its
// actual approved root execution owns the Use. A child inherits its original
// parent's execution and scope even when the provider is another component.
func (b *Bridge) AcquireBinding(ctx context.Context, token, operationID string, binding qimu.BindingSnapshot, parentUseID string) (grant control.UseGrant, returned error) {
	return b.acquireBinding(ctx, token, operationID, binding, nil, parentUseID)
}

// AcquirePlan admits a fresh declaration selection. Its preparation reads stay
// attached until the original Host approves the actual execution.
func (b *Bridge) AcquirePlan(ctx context.Context, token, operationID string, plan *qimu.BindingPlan, parentUseID string) (control.UseGrant, error) {
	binding, err := plan.Snapshot()
	if err != nil {
		return control.UseGrant{}, err
	}
	return b.acquireBinding(ctx, token, operationID, binding, plan, parentUseID)
}

func (b *Bridge) acquireBinding(ctx context.Context, token, operationID string, binding qimu.BindingSnapshot, plan *qimu.BindingPlan, parentUseID string) (grant control.UseGrant, returned error) {
	session, err := b.session(token, true)
	if err != nil {
		return grant, err
	}
	if operationID == "" || len(binding.Roots) != 1 || len(binding.UnitRoots) != 0 {
		return grant, qimu.ErrInvalid
	}
	canonical, err := binding.CanonicalJSON()
	if err != nil {
		return grant, err
	}
	digest, err := requestDigest(struct {
		Binding string
		Parent  string
	}{string(canonical), parentUseID})
	if err != nil {
		return grant, err
	}
	kind := "acquireBinding"
	if plan != nil {
		kind = "acquirePlan"
	}
	b.mu.Lock()
	if original := b.operations[operationID]; original != nil {
		if original.kind != kind || original.realm != session.realm || original.digest != digest {
			b.mu.Unlock()
			return grant, sqlite.ErrConflict
		}
		b.mu.Unlock()
		select {
		case <-ctx.Done():
			return grant, ctx.Err()
		case <-original.done:
		}
		b.mu.Lock()
		defer b.mu.Unlock()
		if original.grant == nil {
			return grant, original.failure
		}
		return clone(*original.grant), nil
	}
	if len(b.operations) >= b.config.MaxPending {
		b.mu.Unlock()
		return grant, qimu.ErrCapacity
	}
	operation := &controlOperation{kind: kind, realm: session.realm, digest: digest, parentID: parentUseID, done: make(chan struct{})}
	b.operations[operationID] = operation
	b.mu.Unlock()
	defer func() { b.mu.Lock(); operation.failure = returned; b.mu.Unlock(); close(operation.done) }()
	if plan != nil {
		_, err = b.config.Host.ActivatePlan(ctx, RealmID(session.realm), plan)
	} else {
		_, err = b.config.Host.ActivateBinding(ctx, RealmID(session.realm), binding)
	}
	if err != nil {
		return grant, err
	}
	var entry *browserExecution
	b.mu.Lock()
	if parentUseID != "" {
		parent := b.uses[parentUseID]
		if parent != nil && parent.grant.Realm == session.realm && parent.releaseID == "" {
			entry = b.executions[parent.grant.Execution.ID]
		}
	} else {
		root := binding.Roots[0].Unit()
		var configuration qimu.Digest
		for _, unit := range binding.Units {
			if unit.Source == root {
				configuration = unit.Configuration
				break
			}
		}
		for _, candidate := range b.executions {
			if candidate.released || candidate.plan.Realm != session.realm || candidate.readyOperation == "" || candidate.attempt.Source != root || candidate.plan.Units[0].Instance.Configuration != configuration {
				continue
			}
			if entry != nil {
				b.mu.Unlock()
				return grant, qimu.ErrAmbiguous
			}
			entry = candidate
		}
	}
	if entry == nil || entry.released || session.cleanupOnly {
		b.mu.Unlock()
		return grant, qimu.ErrStaleBinding
	}
	operation.execution = entry.plan.Execution.ID
	b.mu.Unlock()
	config, err := b.useConfig(session, entry, parentUseID)
	if err != nil {
		return grant, err
	}
	use, err := b.config.Host.AcquireBinding(ctx, binding, config)
	if err != nil {
		return grant, err
	}
	return b.deliverUse(ctx, session, entry, use, operation, "")
}
