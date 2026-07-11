//go:build darwin && cgo

package tray

import (
	"github.com/getlantern/systray"
)

type Callbacks struct {
	InitiallyRunning   bool
	InitiallyAutostart bool
	OnOpen             func()
	OnStart            func()
	OnStop             func()
	OnAutostart        func(enabled bool)
	OnQuit             func()
}

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

func setIcon(running bool) {
	if running {
		systray.SetTitle("▶")
	} else {
		systray.SetTitle("⏸")
	}
}

func Quit() {
	systray.Quit()
}
