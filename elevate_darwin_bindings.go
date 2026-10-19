//go:build darwin && !cli && bindings

package main

// wails build runs the app with the bindings tag to generate the JS bindings;
// it must not open the administrator prompt.
func ensureElevated() {}
