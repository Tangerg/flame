package terminal

import (
	"image"

	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/core/layout"
	"github.com/Tangerg/oolong/ui/core/scene"
)

const (
	transcriptPaneKey  = "transcript"
	promptPaneKey      = "prompt"
	shortShellHeight   = 16
	minimalShellHeight = 8
	minimalShellWidth  = 12
)

type shellDensity uint8

const (
	shellDensityNormal shellDensity = iota + 1
	shellDensityShort
	shellDensityMinimal
)

func densityForShell(width, height int) shellDensity {
	switch {
	case height < minimalShellHeight || width < minimalShellWidth:
		return shellDensityMinimal
	case height <= shortShellHeight:
		return shellDensityShort
	default:
		return shellDensityNormal
	}
}

func (d shellDensity) hidesOptionalPanes() bool {
	return d == shellDensityShort || d == shellDensityMinimal
}

func (d shellDensity) usesMinimalPrompt() bool { return d == shellDensityMinimal }

type shellView struct {
	rows       *scene.Group
	transcript *transcriptView
	prompt     *promptView
}

func newShellView(header *sessionHeader, transcript *transcriptView, activity *activityView, prompts *queueView, status *statusView, authoredPrompt *promptView) *shellView {
	s := &shellView{transcript: transcript, prompt: authoredPrompt}
	s.rows = scene.NewGroup(scene.GroupConfig{
		Axis: layout.Down,
		Items: []scene.Item{
			{Key: "header", Element: passiveElement(header)},
			{Key: transcriptPaneKey, Element: transcript.Element()},
			{Key: "activity", Element: passiveElement(activity)},
			{Key: "queue", Element: passiveElement(prompts)},
			{Key: "status", Element: passiveElement(status)},
			{Key: promptPaneKey, Element: authoredPrompt.Element()},
		},
		Element: scene.ElementConfig{
			Preview: func(event scene.Event) scene.EventResult {
				return scene.EventResult{Consumed: s.TranscriptFocused() && s.transcript.action(event.Input()) == transcriptPrompt && s.FocusPrompt()}
			},
			Handle: func(event scene.Event) scene.EventResult {
				key, ok := event.Input().(input.Key)
				if !ok || !key.Down() || key.Code != input.Tab {
					return scene.EventResult{}
				}
				return scene.EventResult{Consumed: event.FocusNext(key.Mods&input.Shift != 0)}
			},
		},
		Layout: func(space image.Point, items []scene.Item) []image.Rectangle {
			density := densityForShell(space.X, space.Y)
			sizes := []layout.Sizing{layout.Measured(0, 2), layout.Flex(1), layout.Measured(0, activityMaxRows), layout.Measured(0, queueMaxRows), layout.Fixed(1), layout.Measured(4, 9)}
			if density.hidesOptionalPanes() {
				sizes[0], sizes[2], sizes[3] = layout.Fixed(0), layout.Fixed(0), layout.Fixed(0)
			}
			if density.usesMinimalPrompt() {
				sizes[5] = layout.Fixed(1)
			}
			slots := make([]layout.Slot, len(items))
			for i, item := range items {
				slots[i] = layout.Slot{Size: sizes[i], Of: item.Element}
			}
			return (layout.Flow{Axis: layout.Down}).Rects(space, slots)
		},
	})
	return s
}

func (s *shellView) Element() scene.Element  { return s.rows.Element() }
func (s *shellView) PromptFocused() bool     { return s.prompt.Element().FocusedWithin() }
func (s *shellView) TranscriptFocused() bool { return s.transcript.Element().FocusedWithin() }
func (s *shellView) FocusPrompt() bool       { return s.prompt.Element().RequestFocusFirst() == nil }

func (s *shellView) SetTranscript(transcript *transcriptView) {
	s.transcript = transcript
	items := s.rows.Items()
	for i := range items {
		if items[i].Key == transcriptPaneKey {
			items[i].Element = transcript.Element()
		}
	}
	if err := s.rows.Set(items...); err != nil {
		panic(err)
	}
}
