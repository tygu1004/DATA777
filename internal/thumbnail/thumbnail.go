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

const size = 256

type Generator struct {
	cacheDir string
}

func New(cacheDir string) (*Generator, error) {
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return nil, fmt.Errorf("create thumbnail cache dir: %w", err)
	}
	return &Generator{cacheDir: cacheDir}, nil
}

// GetOrGenerate returns the path to a cached size x size JPEG thumbnail for the sample,
// generating and caching it on disk on first request.
func (g *Generator) GetOrGenerate(sampleID int64, srcPath string) (string, error) {
	thumbPath := filepath.Join(g.cacheDir, fmt.Sprintf("%d.jpg", sampleID))
	if _, err := os.Stat(thumbPath); err == nil {
		return thumbPath, nil
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

	thumb := letterbox(img, size)

	tmpPath := thumbPath + ".tmp"
	out, err := os.Create(tmpPath)
	if err != nil {
		return "", fmt.Errorf("create thumbnail file: %w", err)
	}
	if err := jpeg.Encode(out, thumb, &jpeg.Options{Quality: 85}); err != nil {
		out.Close()
		os.Remove(tmpPath)
		return "", fmt.Errorf("encode thumbnail: %w", err)
	}
	if err := out.Close(); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("close thumbnail file: %w", err)
	}
	if err := os.Rename(tmpPath, thumbPath); err != nil {
		return "", fmt.Errorf("finalize thumbnail file: %w", err)
	}

	return thumbPath, nil
}

// letterbox scales img to fit within an edge x edge square (preserving aspect ratio) and
// centers it on a white canvas, so every thumbnail has identical dimensions — this is what
// lets the frontend grid use a fixed-size virtualizer instead of variable-height layout.
func letterbox(img image.Image, edge int) image.Image {
	b := img.Bounds()
	srcW, srcH := b.Dx(), b.Dy()

	scale := float64(edge) / float64(srcW)
	if h := float64(edge) / float64(srcH); h < scale {
		scale = h
	}
	dstW := int(float64(srcW) * scale)
	dstH := int(float64(srcH) * scale)
	if dstW < 1 {
		dstW = 1
	}
	if dstH < 1 {
		dstH = 1
	}

	scaled := image.NewRGBA(image.Rect(0, 0, dstW, dstH))
	draw.CatmullRom.Scale(scaled, scaled.Bounds(), img, b, draw.Over, nil)

	canvas := image.NewRGBA(image.Rect(0, 0, edge, edge))
	draw.Draw(canvas, canvas.Bounds(), image.White, image.Point{}, draw.Src)
	offsetX := (edge - dstW) / 2
	offsetY := (edge - dstH) / 2
	draw.Draw(canvas, image.Rect(offsetX, offsetY, offsetX+dstW, offsetY+dstH), scaled, image.Point{}, draw.Over)

	return canvas
}
