//go:build darwin && !cgo

package main

import "fmt"

// openGUI prints the dashboard URL. It returns a nil channel because there is
// no native window to wait on — only SIGINT stops the proxy.
func openGUI(guiURL string) (<-chan struct{}, error) {
	fmt.Printf("Dashboard: %s\n", guiURL)
	fmt.Println("\nPress Ctrl+C to stop.")
	return nil, nil
}
