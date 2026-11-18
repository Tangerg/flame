package terminal

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/apps/cli/internal/application/workbench"
	"github.com/Tangerg/oolong/ui/core/term"
	"github.com/Tangerg/oolong/ui/uitest"
)

func runUIFromConfig(t *testing.T, config Config) (*uitest.Host, func(), func() *workbench.Store) {
	t.Helper()
	openWorkbench, currentWorkbench := observeTestWorkbench(config.OpenWorkbench)
	config.OpenWorkbench = openWorkbench
	host := uitest.New(t, uitest.Config{Width: 96, Height: 28})
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	config.Host = &handoverTestHost{Host: host}
	go func() {
		done <- Run(ctx, config)
	}()

	var once sync.Once
	stop := func() {
		once.Do(func() {
			cancel()
			if err := <-done; err != nil {
				t.Errorf("terminal session stopped with %v", err)
			}
		})
	}
	t.Cleanup(stop)
	return host, stop, currentWorkbench
}

type handoverTestHost struct {
	*uitest.Host
	handing atomic.Bool
}

func (h *handoverTestHost) Hand(prepare, run func() error) error {
	if !h.handing.CompareAndSwap(false, true) {
		return term.ErrHandoverActive
	}
	defer h.handing.Store(false)
	if prepare != nil {
		if err := prepare(); err != nil {
			return err
		}
	}
	if run != nil {
		return run()
	}
	return nil
}
