package service

import (
	"strings"
	"testing"

	"PicFolderBot/internal/parser"
)

func TestRootDisplayName(t *testing.T) {
	f := NewFlow(&fakeDisk{subdirs: map[string][]string{}}, "disk:/Товары InnoGoods", parser.ParseCaption)
	if got := f.RootDisplayName(); got != "Товары InnoGoods" {
		t.Fatalf("unexpected root display: %s", got)
	}
}

func TestUploadImageAtLevelProduct(t *testing.T) {
	d := &fakeDisk{subdirs: map[string][]string{
		"disk:/Root": {"F05P01"},
	}}
	f := NewFlow(d, "disk:/Root", parser.ParseCaption)
	target, err := f.UploadImageAtLevel(LevelProduct, UploadPayload{
		Product: "F05P01", Filename: "a.jpg", Content: []byte("x"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if target != "disk:/Root/F05P01/a.jpg" {
		t.Fatalf("unexpected target: %s", target)
	}
}

func TestUploadImageAtLevelColorRequiresColor(t *testing.T) {
	d := &fakeDisk{subdirs: map[string][]string{"disk:/Root": {"F05P01"}}}
	f := NewFlow(d, "disk:/Root", parser.ParseCaption)
	_, err := f.UploadImageAtLevel(LevelColor, UploadPayload{
		Product: "F05P01", Filename: "a.jpg", Content: []byte("x"),
	})
	if err == nil || !strings.Contains(err.Error(), "color is required") {
		t.Fatalf("expected color required error, got: %v", err)
	}
}
