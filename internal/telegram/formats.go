package telegram

import (
	"path/filepath"
	"strings"
)

var allowedImageMIMEs = map[string]string{
	"image/jpeg":   ".jpg",
	"image/jpg":    ".jpg",
	"image/png":    ".png",
	"image/webp":   ".webp",
	"image/gif":    ".gif",
	"image/bmp":    ".bmp",
	"image/tiff":   ".tiff",
	"image/x-tiff": ".tiff",
	"image/heic":   ".heic",
	"image/heif":   ".heif",
	"image/avif":   ".avif",
}

var allowedImageExt = map[string]struct{}{
	".jpg":  {},
	".jpeg": {},
	".png":  {},
	".webp": {},
	".gif":  {},
	".bmp":  {},
	".tif":  {},
	".tiff": {},
	".heic": {},
	".heif": {},
	".avif": {},
}

const allowedFormatsText = ".jpg, .jpeg, .png, .webp, .gif, .bmp, .tif, .tiff, .heic, .heif, .avif"

func isAllowedImageMIME(mimeType string) bool {
	_, ok := allowedImageMIMEs[strings.ToLower(strings.TrimSpace(mimeType))]
	return ok
}

func isAllowedImageExtension(filename string) bool {
	ext := strings.ToLower(strings.TrimSpace(filepath.Ext(filename)))
	_, ok := allowedImageExt[ext]
	return ok
}

func extensionByMIME(mimeType string) string {
	if ext, ok := allowedImageMIMEs[strings.ToLower(strings.TrimSpace(mimeType))]; ok {
		return ext
	}
	return ".jpg"
}
