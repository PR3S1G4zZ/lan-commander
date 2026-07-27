package ui

import (
	"runtime"
	"testing"
)

func TestAvailableWithDisplaySet(t *testing.T) {
	t.Setenv("DISPLAY", ":0")
	if !Available() {
		t.Fatal("Available() = false with DISPLAY set, want true")
	}
}

func TestAvailableHeadless(t *testing.T) {
	t.Setenv("DISPLAY", "")
	t.Setenv("WAYLAND_DISPLAY", "")
	got := Available()
	want := runtime.GOOS == "windows"
	if got != want {
		t.Fatalf("Available() = %v on %s without display, want %v", got, runtime.GOOS, want)
	}
}
