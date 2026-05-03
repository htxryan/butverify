package imageopt

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"
)

func testImage() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 3), G: uint8(y * 3), B: uint8((x + y) * 2), A: 255})
		}
	}
	return img
}

func encodeJPEG(t *testing.T, quality int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, testImage(), &jpeg.Options{Quality: quality}); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}
	return buf.Bytes()
}

func encodePNG(t *testing.T, level png.CompressionLevel) []byte {
	t.Helper()
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: level}
	if err := enc.Encode(&buf, testImage()); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func TestOptimizeJPEGUsesQualityWhenSmaller(t *testing.T) {
	original := encodeJPEG(t, 100)
	out, changed, err := OptimizePath("assets/photo.jpg", original, Options{Quality: 40})
	if err != nil {
		t.Fatalf("OptimizePath: %v", err)
	}
	if !changed {
		t.Fatal("expected jpeg to be recompressed")
	}
	if len(out) >= len(original) {
		t.Fatalf("optimized jpeg should be smaller: got %d original %d", len(out), len(original))
	}
	if _, err := jpeg.Decode(bytes.NewReader(out)); err != nil {
		t.Fatalf("optimized jpeg should decode: %v", err)
	}
}

func TestOptimizePNGOnlyReplacesWhenSmaller(t *testing.T) {
	original := encodePNG(t, png.NoCompression)
	out, changed, err := OptimizePath("assets/chart.png", original, Options{Quality: 75})
	if err != nil {
		t.Fatalf("OptimizePath: %v", err)
	}
	if !changed {
		t.Fatal("expected png to be recompressed")
	}
	if len(out) >= len(original) {
		t.Fatalf("optimized png should be smaller: got %d original %d", len(out), len(original))
	}
	if _, err := png.Decode(bytes.NewReader(out)); err != nil {
		t.Fatalf("optimized png should decode: %v", err)
	}
}

func TestOptimizeUnsupportedFormatUnchanged(t *testing.T) {
	original := []byte("body { color: red }")
	out, changed, err := OptimizePath("styles.css", original, Options{Quality: 75})
	if err != nil {
		t.Fatalf("OptimizePath: %v", err)
	}
	if changed {
		t.Fatal("css should not be optimized")
	}
	if !bytes.Equal(out, original) {
		t.Fatalf("unsupported file changed: %q", out)
	}
}

func TestOptimizeRejectsInvalidQuality(t *testing.T) {
	_, _, err := OptimizePath("photo.jpg", encodeJPEG(t, 90), Options{Quality: 101})
	if err == nil || !strings.Contains(err.Error(), "quality") {
		t.Fatalf("expected quality error, got %v", err)
	}
}

func TestOptimizeRejectsImagesOverPixelGuard(t *testing.T) {
	_, _, err := OptimizePath("photo.jpg", encodeJPEG(t, 90), Options{Quality: 75, MaxPixels: 10})
	if err == nil || !strings.Contains(err.Error(), "exceed") {
		t.Fatalf("expected pixel guard error, got %v", err)
	}
}
