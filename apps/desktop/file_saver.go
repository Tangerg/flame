package main

import (
	"bytes"
	"fmt"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type wailsFileSaver struct {
	dialogs *application.DialogManager
	window  application.Window
}

func (w wailsFileSaver) SaveFile(suggestedFilename string, contents []byte) (bool, error) {
	destination, err := w.ChooseFileDestination(suggestedFilename)
	if err != nil || destination == "" {
		return false, err
	}
	if err := publishExport(destination, bytes.NewReader(contents)); err != nil {
		return false, fmt.Errorf("write selected file: %w", err)
	}
	return true, nil
}
func (w wailsFileSaver) ChooseFileDestination(suggestedFilename string) (string, error) {
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
