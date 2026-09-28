package main

import "github.com/wailsapp/wails/v3/pkg/application"

type wailsWorkingDirectoryPicker struct {
	dialogs *application.DialogManager
	window  application.Window
}

func (w wailsWorkingDirectoryPicker) ChooseWorkingDirectory() (string, error) {
	return w.dialogs.OpenFile().
		CanChooseDirectories(true).
		CanChooseFiles(false).
		CanCreateDirectories(true).
		ResolvesAliases(true).
		AttachToWindow(w.window).
		PromptForSingleSelection()
}
