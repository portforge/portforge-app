package main

import (
	"embed"
	"net/http"
	"runtime"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := NewApp()

	// On Linux, gtk_drag_dest_unset (triggered by DisableWebViewDrop:true) removes
	// WebKitGTK's DnD target registration and prevents the drag-data-received /
	// drag-drop GTK signals from firing, breaking file drop entirely. On macOS the
	// native performDragOperation always runs before any DOM event, so we must keep
	// DisableWebViewDrop:true there to stop WKWebView from navigating to the file.
	disableWebViewDrop := runtime.GOOS != "linux"

	err := wails.Run(&options.App{
		Title:  "PortForge",
		Width:  1920,
		Height: 1080,
		AssetServer: &assetserver.Options{
			Assets:  assets,
			Handler: http.StripPrefix("/mediaitems/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if app.metadataPath == "" {
					http.NotFound(w, r)
					return
				}
				http.FileServer(http.Dir(app.metadataPath)).ServeHTTP(w, r)
			})),
		},
		BackgroundColour:         &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:                app.startup,
		EnableDefaultContextMenu: false,
		DragAndDrop: &options.DragAndDrop{
			EnableFileDrop:     true,
			DisableWebViewDrop: disableWebViewDrop,
		},
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
