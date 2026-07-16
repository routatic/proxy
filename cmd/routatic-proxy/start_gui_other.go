//go:build !darwin

package main

import (
	"fmt"
)

func openGUI(guiURL string) error {
	fmt.Printf("Dashboard: %s\n", guiURL)
	fmt.Println("\nPress Ctrl+C to stop.")
	return nil
}
