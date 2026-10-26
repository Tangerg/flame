package bootstrap

import (
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/google/uuid"
)

func newSessionID() string { return session.IDPrefix + uuid.NewString() }
func newRunID() string     { return runs.NewRunID(uuid.NewString()) }
func newSegmentID() string { return runs.NewSegmentID(uuid.NewString()) }
func newItemID() string    { return runs.NewItemID(uuid.NewString()) }
