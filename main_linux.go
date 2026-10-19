//go:build linux && !cli

package main

import "github.com/wailsapp/wails/v2/pkg/options"

func configurePlatformOptions(appOptions *options.App) {
	appOptions.Frameless = true
}

// The Linux launcher script already starts the GUI through pkexec or sudo.
func ensureElevated() {}
