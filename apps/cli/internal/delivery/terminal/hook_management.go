package terminal

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/Tangerg/flame/apps/cli/internal/delivery/format"
	"github.com/Tangerg/flame/apps/cli/internal/domain/workspace"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

type hookScreens struct {
	screenHost
	service Hooks
}

func (h *hookScreens) ShowHooks() {
	h.reader.show(h.hooksReaderQuery())
}

func (h *hookScreens) hooksReaderQuery() *readerQuery {
	workspacePath := h.session.current.Workspace.Path
	return &readerQuery{
		status: "loading lifecycle hooks", follows: followsTopic(protocol.TopicHooksChanged),
		read: func(ctx context.Context) (readerDocument, error) {
			catalog, err := h.service.Catalog(ctx, workspacePath)
			if err != nil {
				return readerDocument{}, err
			}
			return hooksDocument(workspacePath, catalog), nil
		},
	}
}

func hooksDocument(workspacePath string, catalog workspace.HookCatalog) readerDocument {
	detail := fmt.Sprintf("%d hooks · repository trust %t", len(catalog.Hooks), catalog.RepositoryTrusted)
	if catalog.RepositoryRoot != "" {
		detail += format.Separator + catalog.RepositoryRoot
	}
	if len(catalog.Hooks) == 0 {
		return paragraphDocument("Lifecycle hooks", detail, []string{"No global or repository hooks are configured for " + workspacePath + "."})
	}
	sections := make([]toolSection, 0, len(catalog.Hooks)*2)
	for _, hook := range catalog.Hooks {
		state := "inactive"
		if hook.Active {
			state = "active"
		}
		metadata := []string{"scope    " + string(hook.Scope), "state    " + state, "source   " + hook.Source}
		if hook.Matcher != "" {
			metadata = append(metadata, "matcher  "+hook.Matcher)
		}
		if hook.TimeoutMillis > 0 {
			metadata = append(metadata, "timeout  "+strconv.Itoa(hook.TimeoutMillis)+"ms")
		}
		if hook.FailurePolicy != "" {
			metadata = append(metadata, "failure policy  "+string(hook.FailurePolicy))
		}
		actionTitle, action := "Injected context", hook.Inject
		if hook.Command != "" {
			actionTitle, action = "Shell command", hook.Command
		}
		sections = append(sections,
			toolSection{Title: string(hook.Event) + format.Separator + state, Style: toolSectionCode, Text: strings.Join(metadata, "\n")},
			toolSection{Title: actionTitle, Style: toolSectionCode, Text: action},
		)
	}
	return readerDocument{Title: "Lifecycle hooks", Detail: detail, Sections: sections}
}

func (h *hookScreens) PrepareHookTrust(trusted bool) error {
	workspacePath := h.session.current.Workspace.Path
	h.status.note("loading repository hook trust")
	if !h.tasks.session(hookOperation,
		func(ctx context.Context) (workspace.HookCatalog, error) { return h.service.Catalog(ctx, workspacePath) },
		func(catalog workspace.HookCatalog, err error) {
			if err != nil {
				h.message("load repository hooks failed: " + err.Error())
				return
			}
			if catalog.RepositoryRoot == "" {
				h.message("this workspace has no repository root to trust")
				return
			}
			if catalog.RepositoryTrusted == trusted {
				state := "revoked"
				if trusted {
					state = "trusted"
				}
				h.message("repository hooks are already " + state + format.Separator + catalog.RepositoryRoot)
				return
			}
			title, question, action := "Revoke repository hooks", "Disable repository hooks from "+catalog.RepositoryRoot+"?", "Revoke trust"
			if trusted {
				title, question, action = "Trust repository hooks", "Allow reviewed repository hooks from "+catalog.RepositoryRoot+" to execute?", "Trust hooks"
			}
			h.confirm(title, question, action, "", func() { h.setHookTrust(workspacePath, catalog.RepositoryRoot, trusted) })
		},
	) {
		return hookOperation.busy()
	}
	return nil
}

func (h *hookScreens) setHookTrust(workspacePath, repositoryRoot string, trusted bool) {
	presentation := h.session.context
	h.status.note("updating repository hook trust")
	if !h.tasks.admissionMutation(hookOperation,
		func(ctx context.Context) (workspace.HookCatalog, error) {
			if err := h.service.SetRepositoryTrust(ctx, repositoryRoot, trusted); err != nil {
				return workspace.HookCatalog{}, err
			}
			return h.service.Catalog(ctx, workspacePath)
		},
		func(catalog workspace.HookCatalog, err error) {
			if err != nil {
				h.message("update repository hook trust failed: " + err.Error())
				return
			}
			if h.session.context.current(presentation) {
				h.reader.open(h.hooksReaderQuery(), hooksDocument(workspacePath, catalog))
			}
			h.status.note("repository hook trust updated")
		},
	) {
		h.message(hookOperation.busy().Error())
	}
}
