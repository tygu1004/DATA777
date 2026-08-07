package thumbnail

import (
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"

	"golang.org/x/image/draw"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"
)

const (
	gridSize    = 256  // fixed square grid thumbnails
	previewSize = 1600 // lightbox preview, longest edge
)

// Generator resizes and disk-caches one JPEG variant per sample; grid vs. preview are just
// different edge/letterbox configurations of the same type.
type Generator struct {
	cacheDir  string
	edge      int
	letterbox bool
}

func New(cacheDir string) (*Generator, error) {
	return newGenerator(cacheDir, gridSize, true)
}

func NewPreview(cacheDir string) (*Generator, error) {
	return newGenerator(cacheDir, previewSize, false)
}

func newGenerator(cacheDir string, edge int, letterbox bool) (*Generator, error) {
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return nil, fmt.Errorf("create image cache dir: %w", err)
	}
	return &Generator{cacheDir: cacheDir, edge: edge, letterbox: letterbox}, nil
}

func (g *Generator) GetOrGenerate(sampleID int64, srcPath string) (string, error) {
	outPath := filepath.Join(g.cacheDir, fmt.Sprintf("%d.jpg", sampleID))
	if _, err := os.Stat(outPath); err == nil {
		return outPath, nil
	}

	src, err := os.Open(srcPath)
	if err != nil {
		return "", fmt.Errorf("open source image: %w", err)
	}
	defer src.Close()

	img, _, err := image.Decode(src)
	if err != nil {
		return "", fmt.Errorf("decode source image: %w", err)
	}

	var resized image.Image
	if g.letterbox {
		resized = letterbox(img, g.edge)
	} else {
		resized = scaleToFit(img, g.edge)
	}

	tmpPath := outPath + ".tmp"
	out, err := os.Create(tmpPath)
	if err != nil {
		return "", fmt.Errorf("create image file: %w", err)
	}
	if err := jpeg.Encode(out, resized, &jpeg.Options{Quality: 88}); err != nil {
		out.Close()
		os.Remove(tmpPath)
		return "", fmt.Errorf("encode image: %w", err)
	}
	if err := out.Close(); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("close image file: %w", err)
	}
	if err := os.Rename(tmpPath, outPath); err != nil {
		return "", fmt.Errorf("finalize image file: %w", err)
	}

	return outPath, nil
}

// scaleToFit never upscales — a preview meant to judge image quality shouldn't fake sharpness.
func scaleToFit(img image.Image, edge int) image.Image {
	b := img.Bounds()
	srcW, srcH := b.Dx(), b.Dy()

	scale := float64(edge) / float64(srcW)
	if h := float64(edge) / float64(srcH); h < scale {
		scale = h
	}
	if scale > 1 {
		scale = 1
	}

	dstW := max(1, int(float64(srcW)*scale))
	dstH := max(1, int(float64(srcH)*scale))
	if dstW == srcW && dstH == srcH {
		return img
	}

	scaled := image.NewRGBA(image.Rect(0, 0, dstW, dstH))
	draw.CatmullRom.Scale(scaled, scaled.Bounds(), img, b, draw.Over, nil)
	return scaled
}

// letterbox centers img on a white edge x edge canvas so every thumbnail is the same size.
func letterbox(img image.Image, edge int) image.Image {
	b := img.Bounds()
	srcW, srcH := b.Dx(), b.Dy()

	scale := float64(edge) / float64(srcW)
	if h := float64(edge) / float64(srcH); h < scale {
		scale = h
	}
	dstW := max(1, int(float64(srcW)*scale))
	dstH := max(1, int(float64(srcH)*scale))

	scaled := image.NewRGBA(image.Rect(0, 0, dstW, dstH))
	draw.CatmullRom.Scale(scaled, scaled.Bounds(), img, b, draw.Over, nil)

	canvas := image.NewRGBA(image.Rect(0, 0, edge, edge))
	draw.Draw(canvas, canvas.Bounds(), image.White, image.Point{}, draw.Src)
	offsetX := (edge - dstW) / 2
	offsetY := (edge - dstH) / 2
	draw.Draw(canvas, image.Rect(offsetX, offsetY, offsetX+dstW, offsetY+dstH), scaled, image.Point{}, draw.Over)

	return canvas
}
