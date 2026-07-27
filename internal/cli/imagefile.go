package cli

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ImageFileToDataURI reads a local image file and returns a data URI for Meshy APIs.
// Supported: .jpg/.jpeg, .png (and common aliases). Content is base64-encoded.
func ImageFileToDataURI(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("empty image path")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read image %q: %w", path, err)
	}
	if len(data) == 0 {
		return "", fmt.Errorf("image file %q is empty", path)
	}
	mime, err := imageMIME(path, data)
	if err != nil {
		return "", err
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}

func imageMIME(path string, data []byte) (string, error) {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".jpg", ".jpeg":
		return "image/jpeg", nil
	case ".png":
		return "image/png", nil
	case ".webp":
		return "image/webp", nil
	case ".gif":
		return "image/gif", nil
	}
	// sniff magic if extension missing/unknown
	if len(data) >= 3 && data[0] == 0xff && data[1] == 0xd8 && data[2] == 0xff {
		return "image/jpeg", nil
	}
	if len(data) >= 8 && string(data[:8]) == "\x89PNG\r\n\x1a\n" {
		return "image/png", nil
	}
	if ext == "" {
		return "", fmt.Errorf("cannot determine image type for %q (use .jpg/.png)", path)
	}
	return "", fmt.Errorf("unsupported image extension %q (use .jpg, .jpeg, or .png)", ext)
}

// ResolveImageInput returns a Meshy-ready image_url value from either a URL/data URI
// or a local file path. Local paths (existing files, or non-http schemes that look like paths)
// are converted to data URIs.
func ResolveImageInput(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", nil
	}
	if strings.HasPrefix(s, "data:") {
		return s, nil
	}
	if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") {
		return s, nil
	}
	// treat as local path
	return ImageFileToDataURI(s)
}
