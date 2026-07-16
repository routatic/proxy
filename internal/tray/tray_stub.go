//go:build !(darwin && cgo)

package tray

type Callbacks struct {
	InitiallyRunning   bool
	InitiallyAutostart bool
	OnOpen             func()
	OnStart            func()
	OnStop             func()
	OnAutostart        func(enabled bool)
	OnQuit             func()
}

func Run(cb Callbacks) {}

func SetRunning(running bool) {}

func SetAutostart(enabled bool) {}

func Quit() {}
