package terminal

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/Tangerg/flame/apps/cli/internal/domain/workspace"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

// hookScreens audits lifecycle hooks and changes project hook trust.
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
	detail := fmt.Sprintf("%d hooks · project trust %t", len(catalog.Hooks), catalog.ProjectTrusted)
	if catalog.ProjectRoot != "" {
		detail += " · " + catalog.ProjectRoot
	}
	if len(catalog.Hooks) == 0 {
		return paragraphDocument("Lifecycle hooks", detail, []string{"No global or project hooks are configured for " + workspacePath + "."})
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
			toolSection{Title: string(hook.Event) + " · " + state, Style: toolSectionCode, Text: strings.Join(metadata, "\n")},
			toolSection{Title: actionTitle, Style: toolSectionCode, Text: action},
		)
	}
	return readerDocument{Title: "Lifecycle hooks", Detail: detail, Sections: sections}
}

func (h *hookScreens) PrepareHookTrust(trusted bool) error {
	workspacePath := h.session.current.Workspace.Path
	h.status.note("loading project hook trust")
	if !h.tasks.session(hookOperation, false,
		func(ctx context.Context) (workspace.HookCatalog, error) { return h.service.Catalog(ctx, workspacePath) },
		func(catalog workspace.HookCatalog, err error) {
			if err != nil {
				h.message("load project hooks failed: " + err.Error())
				return
			}
			if catalog.ProjectRoot == "" {
				h.message("this workspace has no project root to trust")
				return
			}
			if catalog.ProjectTrusted == trusted {
				state := "revoked"
				if trusted {
					state = "trusted"
				}
				h.message("project hooks are already " + state + " · " + catalog.ProjectRoot)
				return
			}
			title, question, action := "Revoke project hooks", "Disable project hooks from "+catalog.ProjectRoot+"?", "Revoke trust"
			if trusted {
				title, question, action = "Trust project hooks", "Allow reviewed project hooks from "+catalog.ProjectRoot+" to execute?", "Trust hooks"
			}
			h.confirm(title, question, action, "", func() { h.setHookTrust(workspacePath, catalog.ProjectRoot, trusted) })
		},
	) {
		return errors.New("another hook operation is running")
	}
	return nil
}

func (h *hookScreens) setHookTrust(workspacePath, projectRoot string, trusted bool) {
	presentation := h.session.context
	h.status.note("updating project hook trust")
	if !h.tasks.admissionMutation(hookOperation, false,
		func(ctx context.Context) (workspace.HookCatalog, error) {
			if err := h.service.SetProjectTrust(ctx, projectRoot, trusted); err != nil {
				return workspace.HookCatalog{}, err
			}
			return h.service.Catalog(ctx, workspacePath)
		},
		func(catalog workspace.HookCatalog, err error) {
			if err != nil {
				h.message("update project hook trust failed: " + err.Error())
				return
			}
			if h.session.context.current(presentation) {
				h.reader.open(h.hooksReaderQuery(), hooksDocument(workspacePath, catalog))
			}
			h.status.note("project hook trust updated")
		},
	) {
		h.message("another hook operation is running")
	}
}
