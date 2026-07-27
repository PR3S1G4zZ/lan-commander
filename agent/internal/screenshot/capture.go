package screenshot

import (
	"bytes"
	"fmt"
	"image/png"

	kb "github.com/kbinani/screenshot"
)

// CaptureDisplay captures the specified display and returns PNG bytes plus dimensions.
func CaptureDisplay(displayIndex int) (data []byte, width int, height int, err error) {
	n := kb.NumActiveDisplays()
	if displayIndex < 0 || displayIndex >= n {
		return nil, 0, 0, fmt.Errorf("invalid display index %d: only %d display(s) available", displayIndex, n)
	}

	bounds := kb.GetDisplayBounds(displayIndex)

	img, err := kb.CaptureRect(bounds)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("capture failed for display %d: %w", displayIndex, err)
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, 0, 0, fmt.Errorf("PNG encode failed: %w", err)
	}

	return buf.Bytes(), bounds.Dx(), bounds.Dy(), nil
}

// CaptureAll captures the primary display (index 0).
func CaptureAll() (data []byte, width int, height int, err error) {
	return CaptureDisplay(0)
}
