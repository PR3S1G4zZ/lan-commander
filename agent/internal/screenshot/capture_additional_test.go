package screenshot

import (
	"bytes"
	"image/png"
	"strings"
	"testing"

	kb "github.com/kbinani/screenshot"
)

func assertInvalidCaptureResult(t *testing.T, data []byte, width, height int, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("capture unexpectedly succeeded")
	}
	if data != nil {
		t.Fatalf("capture data = %d bytes on error, want nil", len(data))
	}
	if width != 0 || height != 0 {
		t.Fatalf("capture dimensions = %dx%d on error, want 0x0", width, height)
	}
	if !strings.Contains(err.Error(), "invalid display index") && !strings.Contains(err.Error(), "capture failed") {
		t.Fatalf("capture error = %q, want an invalid-index or capture error", err)
	}
}

func skipCaptureDuringRace(t *testing.T) {
	t.Helper()
	if raceEnabled {
		t.Skip("kbinani/screenshot is incompatible with Windows checkptr under -race")
	}
}

func TestCaptureDisplayRejectsNegativeIndexWithoutCapturing(t *testing.T) {
	skipCaptureDuringRace(t)
	data, width, height, err := CaptureDisplay(-1)
	if err == nil || !strings.Contains(err.Error(), "invalid display index -1") {
		if err == nil {
			t.Fatal("negative display index unexpectedly succeeded")
		}
		t.Fatalf("negative display index error = %q, want an invalid-index error", err)
	}
	if data != nil || width != 0 || height != 0 {
		t.Fatalf("negative-index result = (%d bytes, %dx%d), want (nil, 0x0)", len(data), width, height)
	}
}

func TestCaptureDisplayRejectsIndexAtActiveDisplayCount(t *testing.T) {
	skipCaptureDuringRace(t)
	index := kb.NumActiveDisplays()
	data, width, height, err := CaptureDisplay(index)
	if err == nil || !strings.Contains(err.Error(), "invalid display index") {
		t.Fatalf("display index %d result = (%d bytes, %dx%d, %v), want an invalid-index error", index, len(data), width, height, err)
	}
	if data != nil || width != 0 || height != 0 {
		t.Fatalf("out-of-range result = (%d bytes, %dx%d), want (nil, 0x0)", len(data), width, height)
	}
}

func TestCaptureAllReturnsValidPNGOrSafeDesktopError(t *testing.T) {
	skipCaptureDuringRace(t)
	data, width, height, err := CaptureAll()
	if err != nil {
		assertInvalidCaptureResult(t, data, width, height, err)
		t.Skipf("desktop capture is unavailable in this environment: %v", err)
	}
	if width <= 0 || height <= 0 {
		t.Fatalf("capture dimensions = %dx%d, want positive dimensions", width, height)
	}
	if len(data) == 0 {
		t.Fatal("capture returned no PNG data")
	}
	decoded, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode captured PNG: %v", err)
	}
	if got := decoded.Bounds(); got.Dx() != width || got.Dy() != height {
		t.Fatalf("decoded dimensions = %dx%d, want %dx%d", got.Dx(), got.Dy(), width, height)
	}
}
