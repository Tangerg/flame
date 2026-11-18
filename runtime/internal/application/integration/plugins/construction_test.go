package plugins

import (
	"context"

	"github.com/Tangerg/flame/runtime/internal/application/invalidation"

	framehost "github.com/Tangerg/frame/host"
)

func newTestCoordinator(lifetime context.Context, host framehost.Host, store Store, catalog catalog, packages packages, connections connections, publish invalidation.Publish) (*Coordinator, error) {
	admission := framehost.NewAdmission()
	if host.Valid() {
		admission = host.Admission()
	}
	return New(lifetime, host, store, catalog, packages, connections, admission.Admit, publish)
}
