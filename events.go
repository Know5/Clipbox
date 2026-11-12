package main

import "github.com/wailsapp/wails/v2/pkg/runtime"

func (a *App) emit(name string, data ...any) {
	if a.resident != nil {
		a.resident.publish(name, data...)
	} else if a.ctx != nil {
		runtime.EventsEmit(a.ctx, name, data...)
	}
}

func (a *App) startupNotice(options runtime.MessageDialogOptions) {
	if a.resident != nil {
		go messageBox(options.Title, options.Message, mbOK|mbIconWarning)
	} else if a.ctx != nil {
		_, _ = runtime.MessageDialog(a.ctx, options)
	}
}
