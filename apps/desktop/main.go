package main

import (
	"log"

	"github.com/wailsapp/wails/v3/pkg/services/notifications"

	"github.com/Tangerg/flame/apps/desktop/internal/bridge"
	"github.com/wailsapp/wails/v3/pkg/application"
)

func main() {
	host, err := defaultDesktopHost()
	if err != nil {
		log.Fatal(err)
	}
	app := application.New(desktopApplicationOptions(host))
	window := app.Window.NewWithOptions(desktopWindowOptions())
	host.useWindow(window)
	host.usePathRevealer(app.Env)
	host.usePathOpener(app.Browser)
	host.useNotifications(wailsNotifications{
		service: notifications.New(),
		opened: func(target bridge.NotificationTarget) {
			app.Event.Emit(bridge.NotificationOpenedEvent, target)
		},
	})
	host.useRevealer(func() {
		window.UnMinimise()
		window.Show()
		window.Focus()
	})
	host.useWorkingDirectoryPicker(wailsWorkingDirectoryPicker{
		dialogs: app.Dialog,
		window:  window,
	})
	host.useFileSaver(wailsFileSaver{
		dialogs: app.Dialog,
		window:  window,
	})
	installNativeZoom(app)
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
