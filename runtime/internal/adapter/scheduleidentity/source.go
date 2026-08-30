// Package scheduleidentity creates the stable Run identities captured by a
// durable schedule occurrence before it is claimed.
package scheduleidentity

import (
	"github.com/google/uuid"

	"github.com/Tangerg/flame/runtime/internal/application/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/schedule"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
)

// Source is the production schedule-occurrence identity source. Its zero value
// is ready to use.
type Source struct{}

// NewScheduleID returns a namespaced Schedule identity with fresh entropy.
func (Source) NewScheduleID() string { return schedule.IDPrefix + uuid.NewString() }

// NewSessionID returns a namespaced Session identity with fresh entropy.
func (Source) NewSessionID() string { return session.IDPrefix + uuid.NewString() }

// NewRunID returns a namespaced Run identity with fresh entropy.
func (Source) NewRunID() string { return runs.NewRunID(uuid.NewString()) }
