package main

import (
	"embed"
	"log"
	"os"
	"path/filepath"

	"github.com/axeprpr/n2nGUI/internal/app"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed frontend/*
var assets embed.FS

func main() {
	baseDir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	baseDir, err = filepath.Abs(filepath.Join(baseDir, ".."))
	if err != nil {
		log.Fatal(err)
	}

	server := app.NewServer(baseDir)

	err = wails.Run(&options.App{
		Title:         "n2nGUI",
		Width:         1440,
		Height:        960,
		MinWidth:      1160,
		MinHeight:     760,
		DisableResize: false,
		Frameless:     false,
		AssetServer: &assetserver.Options{
			Assets:  assets,
			Handler: server.APIHandler(),
		},
		BackgroundColour: &options.RGBA{R: 7, G: 17, B: 27, A: 1},
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
			IsZoomControlEnabled: false,
		},
	})
	if err != nil {
		log.Fatal(err)
	}
}
