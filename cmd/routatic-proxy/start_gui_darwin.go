//go:build darwin && cgo

package main

import (
	"fmt"

	"github.com/energye/systray"
	"github.com/webview/webview"
)

func openGUI(guiURL string) error {
	fmt.Printf("\nDashboard: %s\n", guiURL)
	fmt.Println("Opening native window...")

	// Set up system tray first (initializes on main thread), then start webview
	// run loop inside onReady so both coexist: tray menu + native webview window.
	systray.Run(func() {
		systray.SetTitle("routatic-proxy")
		systray.SetTooltip("routatic-proxy is running")
		mQuit := systray.AddMenuItem("Quit", "Stop the proxy")

		wv := webview.New(false)
		wv.SetTitle("routatic-proxy")
		wv.SetSize(1200, 800, webview.HintNone)
		wv.Navigate(guiURL)

		go func() {
			<-mQuit.ClickedCh
			wv.Dispatch(func() {
				wv.Terminate()
			})
			systray.Quit()
		}()

		wv.Run()
		wv.Destroy()
		systray.Quit()
	}, nil)

	return nil
}
