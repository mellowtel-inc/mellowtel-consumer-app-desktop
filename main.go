package main

import (
	"embed"
	"os"

	"github.com/rs/zerolog/log"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"mellowtel-consumer/internal/account"
	"mellowtel-consumer/internal/autostart"
	"mellowtel-consumer/internal/config"
	"mellowtel-consumer/internal/device"
	"mellowtel-consumer/internal/logging"
	"mellowtel-consumer/internal/node"
	"mellowtel-consumer/internal/tray"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app, err := bootstrap()
	if err != nil {
		// Logging may not be up yet; print and exit.
		println("fatal:", err.Error())
		os.Exit(1)
	}
	defer func() {
		if app.logCloser != nil {
			app.logCloser.Close()
		}
	}()

	// Build the tray with callbacks bound to the app.
	app.tray = tray.New(log.Logger, tray.Callbacks{
		OnShow:   func() { app.ShowWindow() },
		OnToggle: func() { app.Toggle() },
		OnQuit: func() {
			if app.ctx != nil {
				app.Quit()
			} else {
				os.Exit(0)
			}
		},
	})
	app.tray.Start()

	err = wails.Run(&options.App{
		Title:            "Earnbear",
		Width:            430,
		Height:           700,
		MinWidth:         390,
		MinHeight:        620,
		DisableResize:    false,
		BackgroundColour: &options.RGBA{R: 248, G: 250, B: 252, A: 1},
		AssetServer:      &assetserver.Options{Assets: assets},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		// Let the native window close button hide Earnbear without routing the
		// macOS/Dock Quit command through the same close-to-tray callback.
		HideWindowOnClose: true,
		Bind: []interface{}{
			app,
		},
		Linux: &linux.Options{
			ProgramName: "Earnbear",
		},
		Mac: &mac.Options{
			TitleBar: mac.TitleBarHiddenInset(),
			About: &mac.AboutInfo{
				Title:   "Earnbear",
				Message: "Consensual bandwidth sharing. Version " + config.AppVersion,
			},
		},
	})
	if err != nil {
		log.Error().Err(err).Msg("wails run failed")
	}

	app.tray.Quit()
}

// bootstrap performs non-UI initialisation: config, logging, device identity
// and the node manager.
func bootstrap() (*App, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	configDir, err := config.Dir()
	if err != nil {
		return nil, err
	}
	logPath, closer, err := logging.Setup(configDir, false)
	if err != nil {
		return nil, err
	}
	log.Info().Str("version", config.AppVersion).Str("config", configDir).Msg("starting Mellowtel consumer")

	deviceID, err := device.GetOrCreate(configDir, cfg.Integration)
	if err != nil {
		return nil, err
	}
	log.Info().Str("device_id", deviceID).Msg("node identity ready")

	manager := node.NewManager(log.Logger, cfg, configDir, deviceID)

	execPath, _ := os.Executable()

	return &App{
		cfg:       cfg,
		manager:   manager,
		autostart: autostart.New(execPath),
		account:   account.New(configDir),
		configDir: configDir,
		logPath:   logPath,
		logCloser: closer,
		execPath:  execPath,
	}, nil
}
