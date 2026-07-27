// Package ui implements the client-facing interface that runs in the user's
// desktop session. It is never used by the service process.
package ui

import "os"

// Available reports whether a graphical environment is present. On Linux a
// headless server has no DISPLAY/WAYLAND_DISPLAY, and starting the interface
// there would fail; the service must keep running regardless.
func Available() bool {
	if os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != "" {
		return true
	}
	return isWindows()
}
