//go:build darwin && cgo

package main

import (
	"fmt"
	"os/exec"

	"github.com/routatic/proxy/internal/tray"
)

// openGUI starts the system tray icon and opens the dashboard in the default
// browser. It returns a channel that is closed when the user clicks "Quit" in
// the tray menu, signaling that the proxy should shut down.
//
// systray.Run must be called from the main OS thread on macOS (AppKit
// requirement). The goroutine that calls this function must be the main
// goroutine locked to the OS thread via runtime.LockOSThread, OR the library
// handles it internally (getlantern/systray does).
func openGUI(guiURL string) (<-chan struct{}, error) {
	fmt.Printf("\nDashboard: %s\n", guiURL)

	// Open dashboard in default browser.
	_ = exec.Command("open", guiURL).Start()

	done := make(chan struct{})

	go func() {
		tray.Run(tray.Callbacks{
			InitiallyRunning:   true,
			InitiallyAutostart: false,
			OnOpen: func() {
				_ = exec.Command("open", guiURL).Start()
			},
			OnQuit: func() {
				close(done)
			},
		})
	}()

	return done, nil
}
