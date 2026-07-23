//go:build darwin && cgo

package main

import (
	"fmt"

	"github.com/webview/webview_go"
)

// openGUI opens the dashboard in a native macOS webview. It returns a channel
// that is closed when the user closes the window, signaling that the proxy
// should shut down.
func openGUI(guiURL string) (<-chan struct{}, error) {
	fmt.Printf("\nDashboard: %s\n", guiURL)
	fmt.Println("Opening native window...")

	done := make(chan struct{})
	go func() {
		wv := webview.New(false)
		defer wv.Destroy()
		wv.SetTitle("routatic-proxy")
		wv.SetSize(1200, 800, webview.HintNone)
		wv.Navigate(guiURL)
		wv.Run()
		close(done)
	}()
	return done, nil
}
