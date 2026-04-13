package service

import (
	"errors"
	"fmt"
	"path"
	"strings"

	"PicFolderBot/internal/parser"
)

type CaptionParser func(caption string) parser.ParsedInput

type DiskAPI interface {
	ListSubdirs(diskPath string) ([]string, error)
	EnsureDir(diskPath string) error
	UploadFile(diskPath string, data []byte, mimeType string) error
}

type UploadPayload struct {
	Product  string
	Color    string
	Section  string
	Filename string
	MimeType string
	Content  []byte
}

type Flow struct {
	disk       DiskAPI
	root       string
	parseInput CaptionParser
}

func NewFlow(disk DiskAPI, root string, parseInput CaptionParser) *Flow {
	return &Flow{
		disk:       disk,
		root:       strings.TrimSuffix(strings.TrimSpace(root), "/"),
		parseInput: parseInput,
	}
}

func (f *Flow) ParseCaption(caption string) parser.ParsedInput {
	return f.parseInput(caption)
}

func (f *Flow) ListProducts() ([]string, error) {
	return f.disk.ListSubdirs(f.root)
}

func (f *Flow) ListColors(product string) ([]string, error) {
	product = normalizeToken(product)
	if product == "" {
		return nil, errors.New("product is empty")
	}
	return f.disk.ListSubdirs(joinDisk(f.root, product))
}

func (f *Flow) ListSections(product, color string) ([]string, error) {
	p := normalizeToken(product)
	c := BuildColorFolder(product, color)
	if p == "" || c == "" {
		return nil, errors.New("product or color is empty")
	}
	return f.disk.ListSubdirs(joinDisk(f.root, p, c))
}

func (f *Flow) UploadImage(payload UploadPayload) (string, error) {
	if len(payload.Content) == 0 {
		return "", errors.New("file is empty")
	}

	p := normalizeToken(payload.Product)
	c := BuildColorFolder(payload.Product, payload.Color)
	s := normalizeToken(payload.Section)
	if p == "" || c == "" || s == "" {
		return "", errors.New("product, color and section are required")
	}

	folderPath := joinDisk(f.root, p, c, s)
	if err := f.disk.EnsureDir(folderPath); err != nil {
		return "", err
	}

	filename := strings.TrimSpace(payload.Filename)
	if filename == "" {
		filename = "image.jpg"
	}

	fullPath := joinDisk(folderPath, filename)
	if err := f.disk.UploadFile(fullPath, payload.Content, payload.MimeType); err != nil {
		return "", err
	}
	return fullPath, nil
}

func (f *Flow) CreateFolder(product, color, section, newFolder string) (string, error) {
	p := normalizeToken(product)
	c := BuildColorFolder(product, color)
	s := normalizeToken(section)
	n := normalizeToken(newFolder)

	if p == "" || c == "" || s == "" || n == "" {
		return "", errors.New("all fields are required for folder creation")
	}

	target := joinDisk(f.root, p, c, s, n)
	if err := f.disk.EnsureDir(target); err != nil {
		return "", err
	}
	return target, nil
}

func BuildColorFolder(product, color string) string {
	p := normalizeToken(product)
	c := normalizeToken(color)
	if c == "" {
		return ""
	}
	if p == "" {
		return c
	}
	if strings.HasPrefix(c, p+"-") {
		return c
	}
	return fmt.Sprintf("%s-%s", p, c)
}

func normalizeToken(v string) string {
	return strings.ToUpper(strings.TrimSpace(v))
}

func joinDisk(parts ...string) string {
	if len(parts) == 0 {
		return ""
	}

	root := strings.TrimSuffix(parts[0], "/")
	clean := make([]string, 0, len(parts)-1)
	for _, p := range parts[1:] {
		p = strings.Trim(strings.TrimSpace(p), "/")
		if p != "" {
			clean = append(clean, p)
		}
	}
	if len(clean) == 0 {
		return root
	}
	return root + "/" + path.Join(clean...)
}
