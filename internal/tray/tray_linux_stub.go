//go:build linux && !cgo

// Package tray is a no-op stub for Linux without CGO.
// The full tray implementation requires CGO and ayatana-appindicator3-0.1.
package tray

// Callbacks holds the functions the tray calls when menu items are clicked.
type Callbacks struct {
	InitiallyRunning   bool
	InitiallyAutostart bool
	OnOpen             func()
	OnStart            func()
	OnStop             func()
	OnAutostart        func(enabled bool)
	OnQuit             func()
}

// Run is a no-op when CGO is disabled.
func Run(cb Callbacks) {}

// SetRunning is a no-op.
func SetRunning(running bool) {}

// SetAutostart is a no-op.
func SetAutostart(enabled bool) {}

// Quit is a no-op.
func Quit() {}
