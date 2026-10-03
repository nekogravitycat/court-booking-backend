package storage

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"io"

	"github.com/disintegration/imaging"
)

// ImageProcessor handles image processing like resizing.
type ImageProcessor struct{}

// Bound decoded memory globally across processor instances.
var imageSlots = make(chan struct{}, 2)

const maxImageDimension = 8192
const maxImagePixels = 16_000_000
const maxImageBytes = 5 << 20

// NewImageProcessor creates a new ImageProcessor.
func NewImageProcessor() *ImageProcessor {
	return &ImageProcessor{}
}

// Size is a bounding box for a generated image.
type Size struct {
	Width, Height int
}

// GenerateThumbnail creates a thumbnail from the source image.
// maxWidth and maxHeight define the bounding box for the thumbnail.
// It returns the thumbnail content as a JPEG.
func (p *ImageProcessor) GenerateThumbnail(content io.Reader, maxWidth, maxHeight int) (io.Reader, error) {
	out, err := p.GenerateThumbnails(content, Size{maxWidth, maxHeight})
	if err != nil {
		return nil, err
	}
	return out[0], nil
}

// GenerateThumbnails decodes the source image once and fits it into each of the given bounding
// boxes, returning one JPEG per size in the same order. Every size is resized from the original
// pixels, so the result is identical to calling GenerateThumbnail once per size.
func (p *ImageProcessor) GenerateThumbnails(content io.Reader, sizes ...Size) ([]io.Reader, error) {
	data, err := io.ReadAll(io.LimitReader(content, maxImageBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxImageBytes {
		return nil, fmt.Errorf("image exceeds compressed size limit")
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to decode image configuration: %w", err)
	}
	if config.Width <= 0 || config.Height <= 0 || config.Width > maxImageDimension || config.Height > maxImageDimension || config.Width > maxImagePixels/config.Height {
		return nil, fmt.Errorf("image exceeds dimension or pixel limit")
	}
	for _, size := range sizes {
		if size.Width <= 0 || size.Height <= 0 || size.Width > maxImageDimension || size.Height > maxImageDimension {
			return nil, fmt.Errorf("invalid thumbnail dimensions")
		}
	}
	imageSlots <- struct{}{}
	defer func() { <-imageSlots }()
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %w", err)
	}

	out := make([]io.Reader, len(sizes))
	for i, size := range sizes {
		thumbnail := imaging.Fit(img, size.Width, size.Height, imaging.Lanczos)
		buf := new(bytes.Buffer)
		if err := jpeg.Encode(buf, thumbnail, &jpeg.Options{Quality: 80}); err != nil {
			return nil, fmt.Errorf("failed to encode thumbnail: %w", err)
		}
		out[i] = buf
	}
	return out, nil
}
