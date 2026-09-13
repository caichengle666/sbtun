//go:build !windows

package app

// Linux and other Unix desktop builds currently rely on the OS process model.
func AcquireSingleInstance() (func(), error) { return func() {}, nil }

func ShowSingleInstanceMessage() {}
