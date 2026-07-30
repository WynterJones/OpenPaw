package handlers

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func encodeTestPNG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		t.Fatalf("encode PNG: %v", err)
	}
	return encoded.Bytes()
}

func TestCropTransparentPNGFramesUsesSharedBounds(t *testing.T) {
	first := image.NewNRGBA(image.Rect(0, 0, 12, 12))
	second := image.NewNRGBA(image.Rect(0, 0, 12, 12))
	fill := color.NRGBA{R: 255, G: 80, B: 160, A: 255}

	for y := 4; y < 9; y++ {
		for x := 4; x < 8; x++ {
			first.Set(x, y, fill)
		}
	}
	for y := 3; y < 8; y++ {
		for x := 2; x < 9; x++ {
			second.Set(x, y, fill)
		}
	}

	cropped, err := cropTransparentPNGFrames([][]byte{
		encodeTestPNG(t, first),
		encodeTestPNG(t, second),
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(cropped) != 2 {
		t.Fatalf("cropped frame count = %d, want 2", len(cropped))
	}

	for index, frame := range cropped {
		img, err := png.Decode(bytes.NewReader(frame))
		if err != nil {
			t.Fatalf("decode cropped frame %d: %v", index, err)
		}
		// Shared content bounds are x=2..9, y=3..9; one pixel of padding
		// produces a stable 9x8 canvas for both frames.
		if got := img.Bounds().Size(); got.X != 9 || got.Y != 8 {
			t.Fatalf("cropped frame %d size = %v, want 9x8", index, got)
		}
	}
}

func TestCropTransparentPNGFramesLeavesFullCanvasUntouched(t *testing.T) {
	full := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			full.Set(x, y, color.NRGBA{A: 255})
		}
	}
	original := encodeTestPNG(t, full)

	cropped, err := cropTransparentPNGFrames([][]byte{original}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(cropped) != 1 || !bytes.Equal(cropped[0], original) {
		t.Fatal("full-canvas frame was unnecessarily rewritten")
	}
}
