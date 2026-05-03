// Package imageopt performs small, dependency-free image recompression for
// files that are about to be bundled for upload.
package imageopt

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"path/filepath"
	"strings"
)

const DefaultMaxPixels = 40_000_000

type Options struct {
	Quality   int
	MaxPixels int
}

func OptimizePath(path string, input []byte, opts Options) ([]byte, bool, error) {
	if opts.Quality < 1 || opts.Quality > 100 {
		return nil, false, fmt.Errorf("imageopt: quality must be between 1 and 100")
	}
	maxPixels := opts.MaxPixels
	if maxPixels == 0 {
		maxPixels = DefaultMaxPixels
	}
	format := imageFormatForPath(path)
	switch format {
	case "jpeg":
		out, err := optimizeJPEG(input, opts.Quality, maxPixels)
		if err != nil {
			return nil, false, err
		}
		if len(out) >= len(input) {
			return input, false, nil
		}
		return out, true, nil
	case "png":
		out, err := optimizePNG(input, maxPixels)
		if err != nil {
			return nil, false, err
		}
		if len(out) >= len(input) {
			return input, false, nil
		}
		return out, true, nil
	default:
		return input, false, nil
	}
}

func CanOptimizePath(path string) bool {
	return imageFormatForPath(path) != ""
}

func imageFormatForPath(path string) string {
	switch strings.ToLower(strings.TrimPrefix(filepath.Ext(path), ".")) {
	case "jpg", "jpeg":
		return "jpeg"
	case "png":
		return "png"
	default:
		return ""
	}
}

func checkBounds(data []byte, maxPixels int) error {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("imageopt: decode image config: %w", err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return fmt.Errorf("imageopt: invalid image dimensions %dx%d", cfg.Width, cfg.Height)
	}
	if cfg.Width > maxPixels/cfg.Height {
		return fmt.Errorf("imageopt: image dimensions %dx%d exceed max pixels %d", cfg.Width, cfg.Height, maxPixels)
	}
	return nil
}

func optimizeJPEG(input []byte, quality int, maxPixels int) ([]byte, error) {
	if err := checkBounds(input, maxPixels); err != nil {
		return nil, err
	}
	img, err := jpeg.Decode(bytes.NewReader(input))
	if err != nil {
		return nil, fmt.Errorf("imageopt: decode jpeg: %w", err)
	}
	var out bytes.Buffer
	if err := jpeg.Encode(&out, img, &jpeg.Options{Quality: quality}); err != nil {
		return nil, fmt.Errorf("imageopt: encode jpeg: %w", err)
	}
	return out.Bytes(), nil
}

func optimizePNG(input []byte, maxPixels int) ([]byte, error) {
	if err := checkBounds(input, maxPixels); err != nil {
		return nil, err
	}
	img, err := png.Decode(bytes.NewReader(input))
	if err != nil {
		return nil, fmt.Errorf("imageopt: decode png: %w", err)
	}
	var out bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(&out, img); err != nil {
		return nil, fmt.Errorf("imageopt: encode png: %w", err)
	}
	return out.Bytes(), nil
}
