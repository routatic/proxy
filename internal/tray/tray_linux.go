//go:build linux && cgo

// Package tray provides Linux system tray support using CGO.
// Requires: libappindicator-gtk3-dev or ayatana-appindicator3-dev
//
// On Fedora/RHEL: sudo dnf install libappindicator-gtk3-devel
// On Ubuntu/Debian: sudo apt install libayatana-appindicator3-dev
package tray

// Build with: CGO_ENABLED=1 go build ./cmd/routatic-proxy
// Without CGO, tray_linux_stub.go provides a no-op implementation.

import (
	"github.com/getlantern/systray"
)

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

// Run initialises the system tray and blocks until quit.
func Run(cb Callbacks) {
	systray.Run(func() { onReady(cb) }, func() {})
}

var (
	mStatus    *systray.MenuItem
	mOpen      *systray.MenuItem
	mStart     *systray.MenuItem
	mStop      *systray.MenuItem
	mAutostart *systray.MenuItem
	mQuit      *systray.MenuItem
)

func onReady(cb Callbacks) {
	systray.SetTitle("")
	systray.SetTooltip("routatic-proxy")
	setIcon(false)

	mStatus = systray.AddMenuItem("● Stopped", "")
	mStatus.Disable()
	systray.AddSeparator()

	mOpen = systray.AddMenuItem("Open Console...", "")
	systray.AddSeparator()

	mStart = systray.AddMenuItem("Start Proxy", "")
	mStop = systray.AddMenuItem("Stop Proxy", "")
	mStop.Hide()
	systray.AddSeparator()

	mAutostart = systray.AddMenuItemCheckbox("Start on Boot", "", false)
	systray.AddSeparator()

	mQuit = systray.AddMenuItem("Quit", "")

	SetRunning(cb.InitiallyRunning)
	SetAutostart(cb.InitiallyAutostart)

	go func() {
		for {
			select {
			case <-mOpen.ClickedCh:
				if cb.OnOpen != nil {
					cb.OnOpen()
				}
			case <-mStart.ClickedCh:
				if cb.OnStart != nil {
					cb.OnStart()
				}
			case <-mStop.ClickedCh:
				if cb.OnStop != nil {
					cb.OnStop()
				}
			case <-mAutostart.ClickedCh:
				checked := !mAutostart.Checked()
				if checked {
					mAutostart.Check()
				} else {
					mAutostart.Uncheck()
				}
				if cb.OnAutostart != nil {
					cb.OnAutostart(checked)
				}
			case <-mQuit.ClickedCh:
				systray.Quit()
				if cb.OnQuit != nil {
					cb.OnQuit()
				}
			}
		}
	}()
}

// SetRunning updates the tray menu to reflect proxy running state.
func SetRunning(running bool) {
	if mStatus == nil || mStart == nil || mStop == nil {
		return
	}
	if running {
		setIcon(true)
		mStatus.SetTitle("● Running")
		mStart.Hide()
		mStop.Show()
	} else {
		setIcon(false)
		mStatus.SetTitle("● Stopped")
		mStop.Hide()
		mStart.Show()
	}
}

// SetAutostart updates the autostart checkbox state.
func SetAutostart(enabled bool) {
	if mAutostart == nil {
		return
	}
	if enabled {
		mAutostart.Check()
	} else {
		mAutostart.Uncheck()
	}
}

// setIcon sets a minimal text icon depending on state.
func setIcon(running bool) {
	if running {
		systray.SetTitle("▶")
	} else {
		systray.SetTitle("⏸")
	}
}

// Quit terminates the systray loop.
func Quit() {
	systray.Quit()
}
