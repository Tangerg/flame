package terminal

import (
	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/layout"
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
	rows       *headless.Container
	transcript *transcriptView
	prompt     *promptView
	header     *sessionHeader
	activity   *activityView
	queue      *queueView
	status     *statusView
	density    shellDensity
}

func newShellView(
	header *sessionHeader,
	transcript *transcriptView,
	activity *activityView,
	queue *queueView,
	status *statusView,
	prompt *promptView,
) *shellView {
	shell := &shellView{
		transcript: transcript, prompt: prompt, header: header,
		activity: activity, queue: queue, status: status, density: shellDensityNormal,
	}
	rows := headless.NewContainer(layout.Down, shell.items(shellDensityNormal)...)
	keys := headless.DefaultContainerKeys()
	keys.Bind(headless.FocusNext, input.Chord{Code: input.Character, Rune: ' '})
	rows.Keys = keys
	shell.rows = rows
	shell.focus(promptPaneKey)
	return shell
}

func (s *shellView) Draw(frame headless.Frame) {
	width, height := frame.Size()
	s.setDensity(densityForShell(width, height))
	s.rows.Draw(frame)
}

func (s *shellView) Handle(event input.Event) bool { return s.rows.Handle(event) }

func (s *shellView) Focus(has bool) { s.rows.Focus(has) }

func (s *shellView) PromptFocused() bool { return s.rows.Focused() == s.prompt }

func (s *shellView) TranscriptFocused() bool { return s.rows.Focused() == s.transcript }

func (s *shellView) FocusPrompt() bool { return s.focus(promptPaneKey) }

func (s *shellView) SetTranscript(transcript *transcriptView) {
	s.transcript = transcript
	s.rows.Set(s.items(s.density)...)
}

func (s *shellView) setDensity(density shellDensity) {
	if s.density == density {
		return
	}
	s.density = density
	s.prompt.SetCompact(density.usesMinimalPrompt())
	s.rows.Set(s.items(density)...)
}

func (s *shellView) items(density shellDensity) []headless.Item {
	headerSize := layout.Measured(0, 2)
	activitySize := layout.Measured(0, activityMaxRows)
	queueSize := layout.Measured(0, queueMaxRows)
	promptSize := layout.Measured(4, 9)
	if density.hidesOptionalPanes() {
		headerSize, activitySize, queueSize = layout.Fixed(0), layout.Fixed(0), layout.Fixed(0)
	}
	if density.usesMinimalPrompt() {
		promptSize = layout.Fixed(1)
	}
	return []headless.Item{
		{Key: "header", Size: headerSize, Of: headless.Static{Of: s.header}},
		{Key: transcriptPaneKey, Size: layout.Flex(1), Of: s.transcript},
		{Key: "activity", Size: activitySize, Of: headless.Static{Of: s.activity}},
		{Key: "queue", Size: queueSize, Of: headless.Static{Of: s.queue}},
		{Key: "status", Size: layout.Fixed(1), Of: headless.Static{Of: s.status}},
		{Key: promptPaneKey, Size: promptSize, Of: s.prompt},
	}
}

func (s *shellView) focus(key string) bool {
	for index, item := range s.rows.Items() {
		if item.Key == key {
			return s.rows.Give(index)
		}
	}
	return false
}
