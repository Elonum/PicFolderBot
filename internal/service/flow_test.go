package service

import (
	"testing"

	"PicFolderBot/internal/parser"
)

type fakeDisk struct {
	paths   []string
	subdirs map[string][]string
}

func (f *fakeDisk) ListSubdirs(p string) ([]string, error) { return f.subdirs[p], nil }
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
	d := &fakeDisk{
		subdirs: map[string][]string{
			"disk:/Товары Innogods":                    {"F05P01"},
			"disk:/Товары Innogods/F05P01":             {"F05P01-ЖЁЛТ"},
			"disk:/Товары Innogods/F05P01/F05P01-ЖЁЛТ": {"Титульники"},
		},
	}
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

	want := "disk:/Товары Innogods/F05P01/F05P01-ЖЁЛТ/Титульники/a.jpg"
	if target != want {
		t.Fatalf("target mismatch: got %s want %s", target, want)
	}
}

func TestUploadImageUsesExistingSectionCase(t *testing.T) {
	d := &fakeDisk{
		subdirs: map[string][]string{
			"disk:/Товары Innogods":                     {"F05P01"},
			"disk:/Товары Innogods/F05P01":              {"F05P01-КРАСН"},
			"disk:/Товары Innogods/F05P01/F05P01-КРАСН": {"Рич-Контент"},
		},
	}
	f := NewFlow(d, "disk:/Товары Innogods", parser.ParseCaption)

	target, err := f.UploadImage(UploadPayload{
		Product:  "F05P01",
		Color:    "КРАСН",
		Section:  "РИЧ-КОНТЕНТ",
		Filename: "a.jpg",
		MimeType: "image/jpeg",
		Content:  []byte("x"),
	})
	if err != nil {
		t.Fatalf("upload error: %v", err)
	}

	want := "disk:/Товары Innogods/F05P01/F05P01-КРАСН/Рич-Контент/a.jpg"
	if target != want {
		t.Fatalf("target mismatch: got %s want %s", target, want)
	}
}

func TestUploadImageFailsWhenSectionDoesNotExist(t *testing.T) {
	d := &fakeDisk{
		subdirs: map[string][]string{
			"disk:/Товары Innogods":                     {"F05P01"},
			"disk:/Товары Innogods/F05P01":              {"F05P01-КРАСН"},
			"disk:/Товары Innogods/F05P01/F05P01-КРАСН": {"Рич-Контент"},
		},
	}
	f := NewFlow(d, "disk:/Товары Innogods", parser.ParseCaption)

	_, err := f.UploadImage(UploadPayload{
		Product:  "F05P01",
		Color:    "КРАСН",
		Section:  "Титульники",
		Filename: "a.jpg",
		MimeType: "image/jpeg",
		Content:  []byte("x"),
	})
	if err == nil {
		t.Fatalf("expected error when section does not exist")
	}
}

func TestCreateFolderAtLevelColor(t *testing.T) {
	d := &fakeDisk{
		subdirs: map[string][]string{
			"disk:/Товары Innogods": {"F05P01"},
		},
	}
	f := NewFlow(d, "disk:/Товары Innogods", parser.ParseCaption)

	target, err := f.CreateFolderAtLevel(LevelColor, "F05P01", "", "", "НовыйЦвет")
	if err != nil {
		t.Fatalf("create folder error: %v", err)
	}

	want := "disk:/Товары Innogods/F05P01/НовыйЦвет"
	if target != want {
		t.Fatalf("target mismatch: got %s want %s", target, want)
	}
}

func TestListSectionsForCustomColorFolderWithoutPrefix(t *testing.T) {
	d := &fakeDisk{
		subdirs: map[string][]string{
			"disk:/Товары Innogods":                  {"F05P01"},
			"disk:/Товары Innogods/F05P01":           {"НовыйЦвет"},
			"disk:/Товары Innogods/F05P01/НовыйЦвет": {"Титульники"},
		},
	}
	f := NewFlow(d, "disk:/Товары Innogods", parser.ParseCaption)

	sections, err := f.ListSections("F05P01", "НовыйЦвет")
	if err != nil {
		t.Fatalf("list sections error: %v", err)
	}
	if len(sections) != 1 || sections[0] != "Титульники" {
		t.Fatalf("unexpected sections: %#v", sections)
	}
}
