package api

import (
	"archive/zip"
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

const maxPreviewBytes = 32 << 20 // 32 MiB

// ExtractDrawingPreview opens a .f2d file (ZIP archive) and returns the bytes
// of the embedded preview image. Searches for candidates in priority order:
// thumbnail.png, preview.png, thumbnail.jpg, preview.jpg, then any .png or .jpg
// at the root of the archive. Content type is inferred from the filename.
func ExtractDrawingPreview(f2dPath string) (data []byte, contentType string, err error) {
	r, err := zip.OpenReader(f2dPath)
	if err != nil {
		return nil, "", fmt.Errorf("open .f2d: %w", err)
	}
	defer r.Close()

	candidates := []string{
		"thumbnail.png",
		"preview.png",
		"thumbnail.jpg",
		"preview.jpg",
	}

	// Check priority-order candidates first
	for _, name := range candidates {
		for _, f := range r.File {
			if f.Name == name {
				return readZipEntry(f)
			}
		}
	}

	// Fall back to any .png/.jpg at root
	for _, f := range r.File {
		dir, name := filepath.Split(f.Name)
		if dir == "" || dir == "/" {
			if strings.HasSuffix(name, ".png") || strings.HasSuffix(name, ".jpg") {
				return readZipEntry(f)
			}
		}
	}

	return nil, "", fmt.Errorf("no preview image found in .f2d archive")
}

func readZipEntry(f *zip.File) (data []byte, contentType string, err error) {
	rc, err := f.Open()
	if err != nil {
		return nil, "", fmt.Errorf("open zip entry %q: %w", f.Name, err)
	}
	defer rc.Close()

	data, err = io.ReadAll(io.LimitReader(rc, maxPreviewBytes+1))
	if err != nil {
		return nil, "", fmt.Errorf("read zip entry %q: %w", f.Name, err)
	}
	if len(data) > maxPreviewBytes {
		return nil, "", fmt.Errorf("preview image exceeds %d bytes", maxPreviewBytes)
	}

	contentType = "image/png"
	if strings.HasSuffix(f.Name, ".jpg") || strings.HasSuffix(f.Name, ".jpeg") {
		contentType = "image/jpeg"
	}

	return data, contentType, nil
}
