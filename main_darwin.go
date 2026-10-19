//go:build darwin && !cli

package main

import (
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
)

func configurePlatformOptions(appOptions *options.App) {
	// No menu bar icon on macOS, so closing the window quits and stops TUN.
	appOptions.HideWindowOnClose = false
	appOptions.Mac = &mac.Options{TitleBar: mac.TitleBarHidden()}
}
