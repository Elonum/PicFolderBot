package telegram

import "testing"

func TestAllowedFormats(t *testing.T) {
	if !isAllowedImageMIME("image/jpeg") {
		t.Fatalf("jpeg mime should be allowed")
	}
	if !isAllowedImageExtension("photo.HEIC") {
		t.Fatalf("heic extension should be allowed")
	}
	if isAllowedImageExtension("malware.exe") {
		t.Fatalf("exe should not be allowed")
	}
}

func TestBuildFileNameSanitizesExtension(t *testing.T) {
	got := buildFileName("unsafe.exe", "image/png")
	if got != "unsafe.png" {
		t.Fatalf("unexpected filename: %s", got)
	}
}
