package main

import (
	"embed"
	"net/http"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := NewApp()

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
			DisableWebViewDrop: true,
		},
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
