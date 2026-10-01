package storage

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/png"
	"testing"
)

func TestThumbnailRejectsOversizedHeaderBeforeDecode(t *testing.T) {
	// An IHDR-only PNG lets DecodeConfig succeed without allocating pixels.
	for _, dimensions := range [][2]uint32{{8193, 1}, {8000, 8000}, {65535, 65535}} {
		var data bytes.Buffer
		data.WriteString("\x89PNG\r\n\x1a\n")
		header := make([]byte, 13)
		binary.BigEndian.PutUint32(header, dimensions[0])
		binary.BigEndian.PutUint32(header[4:], dimensions[1])
		header[8], header[9] = 8, 2
		binary.Write(&data, binary.BigEndian, uint32(13))
		data.WriteString("IHDR")
		data.Write(header)
		binary.Write(&data, binary.BigEndian, crc32.ChecksumIEEE(append([]byte("IHDR"), header...)))
		_, err := NewImageProcessor().GenerateThumbnail(&data, 100, 100)
		if err == nil || !bytes.Contains([]byte(err.Error()), []byte("pixel limit")) {
			t.Fatalf("dimensions=%v err=%v", dimensions, err)
		}
	}
}

func TestThumbnailAcceptsSmallImage(t *testing.T) {
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewNRGBA(image.Rect(0, 0, 20, 10))); err != nil {
		t.Fatal(err)
	}
	result, err := NewImageProcessor().GenerateThumbnail(&data, 10, 10)
	if err != nil {
		t.Fatal(err)
	}
	cfg, format, err := image.DecodeConfig(result)
	if err != nil || format != "jpeg" || cfg.Width != 10 || cfg.Height != 5 {
		t.Fatalf("config=%v format=%s err=%v", cfg, format, err)
	}
}
