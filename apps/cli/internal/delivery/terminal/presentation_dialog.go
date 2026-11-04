package terminal

import (
	"github.com/Tangerg/oolong/ui/base"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
	"github.com/Tangerg/oolong/ui/kit"
)

type presentationIdentity struct {
	retired bool
}

func (p *presentationIdentity) retire() {
	if p != nil {
		p.retired = true
	}
}

func (p *presentationIdentity) current(candidate *presentationIdentity) bool {
	return p != nil && p == candidate && !p.retired
}

// presentationDialog owns the business identity captured by asynchronous answers.
// Renewal preserves the native attachment, but input must first see a frame for
// that presentation. Scene still owns focus, capture and attachment generations.
type presentationDialog struct {
	dialog    *kit.Dialog
	workspace *workspaceScene
	identity  *presentationIdentity
	presented scene.Snapshot[*presentationIdentity]
}

func newPresentationDialog(workspace *workspaceScene, config kit.DialogConfig) *presentationDialog {
	p := &presentationDialog{workspace: workspace}
	body := scene.NewGroup(scene.GroupConfig{
		Items: []scene.Item{{Element: config.Body, Size: layout.Flex(1)}},
		Element: scene.ElementConfig{
			Paint: func(frame scene.Frame) { p.presented.Stage(frame, p.identity) },
			Preview: func(event scene.Event) scene.EventResult {
				shown, valid := p.presented.Value(), true
				if _, pointer := event.Input().(input.Mouse); pointer {
					shown, valid = event.Read(&p.presented)
				}
				if !valid || !p.identity.current(shown) {
					return scene.EventResult{Consumed: true}
				}
				if workspace.preview != nil {
					return workspace.preview(event)
				}
				return scene.EventResult{}
			},
		},
	})
	config.Body = body.Element()
	dismissed := config.Dismissed
	config.Dismissed = func() {
		p.identity.retire()
		workspace.dismissDialog(p.dialog.Element())
		if dismissed != nil {
			dismissed()
		}
	}
	p.dialog = kit.NewDialog(config)
	return p
}

func (p *presentationDialog) Show() {
	if p == nil || p.dialog == nil {
		return
	}
	p.identity.retire()
	p.identity = &presentationIdentity{}
	p.dialog.Controller().Show()
	p.dialog.Element().Invalidate()
	p.workspace.showDialog(p.dialog.Element())
}

func (p *presentationDialog) Dismiss() {
	if p != nil && p.dialog != nil {
		p.dialog.Controller().Dismiss()
	}
}

func (p *presentationDialog) Identity() *presentationIdentity {
	if !p.Open() {
		return nil
	}
	return p.identity
}

func (p *presentationDialog) Open() bool {
	return p != nil && p.dialog != nil && p.dialog.Controller().Open()
}

func (p *presentationDialog) Controller() *base.Dialog {
	if p == nil || p.dialog == nil {
		return nil
	}
	return p.dialog.Controller()
}
