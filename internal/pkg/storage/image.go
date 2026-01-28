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

// NewImageProcessor creates a new ImageProcessor.
func NewImageProcessor() *ImageProcessor {
	return &ImageProcessor{}
}

// GenerateThumbnail creates a thumbnail from the source image.
// maxWidth and maxHeight define the bounding box for the thumbnail.
// It returns the thumbnail content as a JPEG.
func (p *ImageProcessor) GenerateThumbnail(content io.Reader, maxWidth, maxHeight int) (io.Reader, error) {
	// Decode original image
	img, _, err := image.Decode(content)
	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %w", err)
	}

	// Resize image
	thumbnail := imaging.Fit(img, maxWidth, maxHeight, imaging.Lanczos)

	// Encode thumbnail to JPEG
	buf := new(bytes.Buffer)
	if err := jpeg.Encode(buf, thumbnail, &jpeg.Options{Quality: 80}); err != nil {
		return nil, fmt.Errorf("failed to encode thumbnail: %w", err)
	}

	return buf, nil
}
