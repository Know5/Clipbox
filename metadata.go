package main

const (
	appName          = "ClipBox"
	appVersion       = "0.4.1"
	singleInstanceID = "com.know5.clipbox"
	// mainWindowClass 写进 Wails 选项，按类名加标题找回自己的主窗口（windowutil.FindOwnWindow）。
	mainWindowClass = "ClipBoxWindow"

	// 窗口默认尺寸（Wails 启动参数）。
	defaultWindowWidth  = 400
	defaultWindowHeight = 560
)
