package service

import (
	"testing"

	"PicFolderBot/internal/parser"
)

type fakeDisk struct {
	paths []string
}

func (f *fakeDisk) ListSubdirs(_ string) ([]string, error) { return nil, nil }
func (f *fakeDisk) EnsureDir(diskPath string) error {
	f.paths = append(f.paths, diskPath)
	return nil
}
func (f *fakeDisk) UploadFile(diskPath string, _ []byte, _ string) error {
	f.paths = append(f.paths, diskPath)
	return nil
}

func TestBuildColorFolder(t *testing.T) {
	got := BuildColorFolder("F05P01", "ЖЁЛТ")
	if got != "F05P01-ЖЁЛТ" {
		t.Fatalf("unexpected color folder: %s", got)
	}
}

func TestUploadImageBuildsPath(t *testing.T) {
	d := &fakeDisk{}
	f := NewFlow(d, "disk:/Товары Innogods", parser.ParseCaption)

	target, err := f.UploadImage(UploadPayload{
		Product:  "F05P01",
		Color:    "ЖЁЛТ",
		Section:  "Титульники",
		Filename: "a.jpg",
		MimeType: "image/jpeg",
		Content:  []byte("x"),
	})
	if err != nil {
		t.Fatalf("upload error: %v", err)
	}

	want := "disk:/Товары Innogods/F05P01/F05P01-ЖЁЛТ/ТИТУЛЬНИКИ/a.jpg"
	if target != want {
		t.Fatalf("target mismatch: got %s want %s", target, want)
	}
}
