package main

import (
	"embed"
	"fmt"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"

	"github.com/efzynx/sweep/internal/gui"
)

var (
	Version   = "1.0.1-beta"
	Commit    = "none"
	BuildDate = "unknown"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := gui.NewApp()
	app.SetVersion(Version)

	err := wails.Run(&options.App{
		Title:     "Sweep",
		Width:     1140,
		Height:    720,
		MinWidth:  800,
		MinHeight: 560,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 12, G: 16, B: 23, A: 255}, // #0c1017 Nordic background
		OnStartup:        app.Startup,
		OnShutdown:       app.Shutdown,
		Bind: []interface{}{
			app,
		},
		Linux: &linux.Options{
			ProgramName: "sweep-gui",
		},
	})

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error launching Sweep GUI: %v\n", err)
		os.Exit(1)
	}
}
