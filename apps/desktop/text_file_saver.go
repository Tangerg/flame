package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type wailsTextFileSaver struct {
	dialogs *application.DialogManager
	window  application.Window
}

// The host has already validated the name, so its extension is .md or .json.
func (w wailsTextFileSaver) SaveTextFile(suggestedFilename string, contents []byte) (bool, error) {
	extension := filepath.Ext(suggestedFilename)
	filterName := "JSON Files"
	if extension == ".md" {
		filterName = "Markdown Files"
	}
	destination, err := w.dialogs.SaveFile().
		CanCreateDirectories(true).
		AllowsOtherFileTypes(false).
		SetFilename(suggestedFilename).
		AddFilter(filterName, "*"+extension).
		AttachToWindow(w.window).
		PromptForSingleSelection()
	if err != nil {
		return false, fmt.Errorf("open save dialog: %w", err)
	}
	if destination == "" {
		return false, nil
	}
	if err := os.WriteFile(destination, contents, 0o666); err != nil {
		return false, fmt.Errorf("write selected text file: %w", err)
	}
	return true, nil
}
