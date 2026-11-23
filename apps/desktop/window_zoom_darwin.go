package main

import "github.com/wailsapp/wails/v3/pkg/application"

func installNativeZoom(app *application.App) {
	menu := application.DefaultApplicationMenu()
	overrides := map[application.Role]pageZoomChange{
		application.ResetZoom: pageZoomReset,
		application.ZoomIn:    pageZoomIn,
		application.ZoomOut:   pageZoomOut,
	}
	for role, change := range overrides {
		item := menu.FindByRole(role)
		if item == nil {
			continue
		}
		item.OnClick(func(*application.Context) {
			window := app.Window.Current()
			if window == nil {
				return
			}
			changeNativePageZoom(window.NativeWindow(), change)
		})
	}
	app.Menu.Set(menu)
}
