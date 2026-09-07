package main

import (
	"embed"
	"flag"
	"fmt"
	"net/http"
	"os"
	"runtime"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// Server mode exists for systems whose webview renders the UI badly; on
	// Linux that is WebKitGTK, which the desktop build has no way around.
	server := flag.Bool("server", false, "serve the UI over HTTP instead of opening a window, and print the URL")
	addr := flag.String("addr", "127.0.0.1:34116", "address to serve on with -server")
	flag.Parse()

	app := NewApp()

	if *server {
		if err := runServer(app, *addr, assets); err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(1)
		}
		return
	}

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
			Assets: assets,
			Handler: http.StripPrefix("/mediaitems/", artworkHandler(
				func() string { return app.metadataPath },
				thumbCacheDir(),
			)),
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
