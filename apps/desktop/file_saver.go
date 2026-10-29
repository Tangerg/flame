package main

import (
	"fmt"
	"os"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type wailsFileSaver struct {
	dialogs *application.DialogManager
	window  application.Window
}

func (w wailsFileSaver) SaveFile(suggestedFilename string, contents []byte) (bool, error) {
	destination, err := w.dialogs.SaveFile().
		CanCreateDirectories(true).
		AllowsOtherFileTypes(true).
		SetFilename(suggestedFilename).
		AttachToWindow(w.window).
		PromptForSingleSelection()
	if err != nil {
		return false, fmt.Errorf("open save dialog: %w", err)
	}
	if destination == "" {
		return false, nil
	}
	if err := os.WriteFile(destination, contents, 0o666); err != nil {
		return false, fmt.Errorf("write selected file: %w", err)
	}
	return true, nil
}
