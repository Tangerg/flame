package terminal

import (
	"context"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
)

type sessionCatalog struct {
	items      []conversation.Session
	cursors    []string
	nextCursor string
}

func (c sessionCatalog) withPage(page conversation.SessionPage) (sessionCatalog, error) {
	cursors := append(slices.Clone(c.cursors), c.nextCursor)
	if page.NextCursor != "" && slices.Contains(cursors, page.NextCursor) {
		return sessionCatalog{}, fmt.Errorf("session catalog returned cyclic continuation cursor %q", page.NextCursor)
	}
	seen := make(map[string]struct{}, len(c.items)+len(page.Items))
	for _, item := range c.items {
		seen[item.ID] = struct{}{}
	}
	for _, item := range page.Items {
		if _, duplicate := seen[item.ID]; duplicate {
			return sessionCatalog{}, fmt.Errorf("session page repeats previously loaded id %q", item.ID)
		}
		seen[item.ID] = struct{}{}
	}
	return sessionCatalog{
		items: append(slices.Clone(c.items), page.Items...), cursors: cursors, nextCursor: page.NextCursor,
	}, nil
}

type sessionCatalogIntent uint8

const (
	openSessionCatalog sessionCatalogIntent = iota
	extendSessionCatalog
	refreshSessionCatalog
)

func (b *sessionBrowser) ShowSessions() {
	if b.observing() {
		b.notify("finish or cancel the current run before switching sessions")
		return
	}
	if !b.dialog.Open() {
		b.loadSessionCatalog(openSessionCatalog)
	}
}

func (b *sessionBrowser) loadMoreSessions() {
	if b.pane.catalog.nextCursor == "" {
		b.notify("all sessions are already loaded")
		return
	}
	b.loadSessionCatalog(extendSessionCatalog)
}

func (b *sessionBrowser) refreshOpenCatalog() {
	if b.dialog.Open() {
		b.loadSessionCatalog(refreshSessionCatalog)
	}
}

func (b *sessionBrowser) dismissCatalog() {
	b.tasks.operations.Cancel(sessionCatalogOperation)
	b.catalogRefreshPending = false
	b.dialog.Dismiss()
}

func (b *sessionBrowser) loadSessionCatalog(intent sessionCatalogIntent) {
	active := b.tasks.operations.Active(sessionCatalogOperation)
	replacingRefresh := active && intent == extendSessionCatalog && b.catalogIntent == refreshSessionCatalog
	if active && !replacingRefresh {
		if intent == refreshSessionCatalog {
			b.catalogRefreshPending = true
		} else {
			b.notify(sessionCatalogOperation.busy().Error())
		}
		return
	}
	var base sessionCatalog
	depth := 1
	switch intent {
	case openSessionCatalog:
		b.pane.Reset()
	case extendSessionCatalog:
		if replacingRefresh {
			depth = len(b.pane.catalog.cursors) + 1
		} else {
			base = b.pane.catalog
		}
	case refreshSessionCatalog:
		depth = len(b.pane.catalog.cursors)
	}
	if intent != refreshSessionCatalog {
		b.notify("loading sessions")
	}
	slot := sessionCatalogOperation
	if replacingRefresh {
		slot = slot.superseding()
	}
	b.catalogRefreshPending = false
	b.catalogIntent = intent
	b.tasks.application(slot,
		func(ctx context.Context) (sessionCatalog, error) {
			return b.readSessionCatalog(ctx, base, depth)
		},
		func(catalog sessionCatalog, err error) {
			if err != nil {
				b.notify("could not load sessions: " + err.Error())
			} else {
				b.pane.SetCatalog(catalog)
				if intent == openSessionCatalog {
					b.dialog.Show()
				}
				if intent != refreshSessionCatalog {
					b.status.note("choose a session")
				}
			}
			if b.catalogRefreshPending {
				b.refreshOpenCatalog()
			}
		},
	)
}

func (b *sessionBrowser) readSessionCatalog(ctx context.Context, catalog sessionCatalog, depth int) (sessionCatalog, error) {
	for range depth {
		page, err := b.runtime.ListSessions(ctx, conversation.SessionQuery{
			PageSize: conversation.DefaultPageSize(), Cursor: catalog.nextCursor,
		})
		if err != nil {
			return sessionCatalog{}, err
		}
		catalog, err = catalog.withPage(page)
		if err != nil {
			return sessionCatalog{}, err
		}
		if catalog.nextCursor == "" {
			break
		}
	}
	return catalog, nil
}
