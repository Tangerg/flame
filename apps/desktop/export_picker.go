package main

import (
	"fmt"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type wailsExportPicker struct {
	dialogs *application.DialogManager
	window  application.Window
}

func (w wailsExportPicker) ChooseFileDestination(suggestedFilename string) (string, error) {
	destination, err := w.dialogs.SaveFile().
		CanCreateDirectories(true).
		AllowsOtherFileTypes(true).
		SetFilename(suggestedFilename).
		AttachToWindow(w.window).
		PromptForSingleSelection()
	if err != nil {
		return "", fmt.Errorf("open save dialog: %w", err)
	}
	return destination, nil
}
