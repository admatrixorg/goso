// Copyright (c) 2026 MQ Global — GOSO Desktop. Clean-room implementation.
//go:build wails

package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := NewApp()

	err := wails.Run(&options.App{
		Title:  "GOSO",
		Width:  1100,
		Height: 800,
		AssetServer: &assetserver.Options{
			Assets:     assets,
			Middleware: app.middleware(),
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		Bind:             []interface{}{app},
		Mac: &mac.Options{
			TitleBar: mac.TitleBarDefault(),
			About: &mac.AboutInfo{
				Title:   "GOSO",
				Message: "GOSO Desktop — Control Plane + local gateway (SQLite)",
			},
		},
	})
	if err != nil {
		log.Fatal(err)
	}
}
