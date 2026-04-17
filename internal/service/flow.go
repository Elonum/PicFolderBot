package service

import (
	"errors"
	"fmt"
	"path"
	"strings"
	"time"

	"PicFolderBot/internal/cache"
	"PicFolderBot/internal/observability"
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

const (
	LevelProduct = "product"
	LevelColor   = "color"
	LevelSection = "section"
)

type Flow struct {
	disk       DiskAPI
	root       string
	parseInput CaptionParser
	cache      cache.TreeCache
}

type Option func(*Flow)

func WithTreeCache(tree cache.TreeCache) Option {
	return func(f *Flow) {
		f.cache = tree
	}
}

func NewFlow(disk DiskAPI, root string, parseInput CaptionParser, opts ...Option) *Flow {
	flow := &Flow{
		disk:       disk,
		root:       strings.TrimSuffix(strings.TrimSpace(root), "/"),
		parseInput: parseInput,
	}
	for _, opt := range opts {
		opt(flow)
	}
	return flow
}

func (f *Flow) ParseCaption(caption string) parser.ParsedInput {
	return f.parseInput(caption)
}

func (f *Flow) RootDisplayName() string {
	root := strings.TrimSpace(f.root)
	if root == "" {
		return "disk"
	}

	trimmed := strings.TrimSuffix(root, "/")
	if idx := strings.LastIndex(trimmed, "/"); idx >= 0 && idx < len(trimmed)-1 {
		return strings.TrimSpace(trimmed[idx+1:])
	}
	if strings.EqualFold(trimmed, "disk:") {
		return "disk"
	}
	return strings.TrimSuffix(trimmed, ":")
}

func (f *Flow) ListProducts() ([]string, error) {
	start := time.Now()
	defer func() { observability.ObserveListProducts(time.Since(start)) }()
	return f.listCached(f.root)
}

func (f *Flow) ListColors(product string) ([]string, error) {
	start := time.Now()
	defer func() { observability.ObserveListColors(time.Since(start)) }()
	product = normalizeToken(product)
	if product == "" {
		return nil, errors.New("product is empty")
	}
	resolved, err := f.resolveExistingName(f.root, product)
	if err != nil {
		return nil, err
	}
	if resolved != "" {
		product = resolved
	}
	return f.listCached(joinDisk(f.root, product))
}

func (f *Flow) ListSections(product, color string) ([]string, error) {
	start := time.Now()
	defer func() { observability.ObserveListSections(time.Since(start)) }()
	p := normalizeToken(product)
	c := normalizeToken(color)
	if p == "" || c == "" {
		return nil, errors.New("product or color is empty")
	}
	if resolved, e := f.resolveExistingName(f.root, p); e != nil {
		return nil, e
	} else if resolved != "" {
		p = resolved
	}
	resolvedColor, err := f.resolveColorFolder(joinDisk(f.root, p), p, c)
	if err != nil {
		return nil, err
	}
	if resolvedColor != "" {
		c = resolvedColor
	} else {
		c = BuildColorFolder(p, c)
	}
	return f.listCached(joinDisk(f.root, p, c))
}

func (f *Flow) UploadImage(payload UploadPayload) (string, error) {
	return f.UploadImageAtLevel(LevelSection, payload)
}

func (f *Flow) UploadImageAtLevel(level string, payload UploadPayload) (string, error) {
	if len(payload.Content) == 0 {
		return "", errors.New("file is empty")
	}

	p := normalizeToken(payload.Product)
	c := normalizeToken(payload.Color)
	s := normalizeToken(payload.Section)
	if p == "" {
		return "", errors.New("product is required")
	}

	if resolved, err := f.resolveExistingName(f.root, p); err != nil {
		return "", err
	} else if resolved != "" {
		p = resolved
	} else {
		return "", fmt.Errorf("product folder not found: %s", p)
	}

	folderPath := joinDisk(f.root, p)
	if level == LevelColor || level == LevelSection {
		if c == "" {
			return "", errors.New("color is required")
		}
		resolvedColor, err := f.resolveColorFolder(joinDisk(f.root, p), p, c)
		if err != nil {
			return "", err
		}
		if resolvedColor != "" {
			c = resolvedColor
		} else {
			return "", fmt.Errorf("color folder not found: %s", c)
		}
		folderPath = joinDisk(f.root, p, c)
	}
	if level == LevelSection {
		if s == "" {
			return "", errors.New("section is required")
		}
		if resolved, err := f.resolveExistingName(joinDisk(f.root, p, c), s); err != nil {
			return "", err
		} else if resolved != "" {
			s = resolved
		} else {
			return "", fmt.Errorf("section folder not found: %s", s)
		}
		folderPath = joinDisk(f.root, p, c, s)
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

func (f *Flow) CreateFolderAtLevel(level, product, color, section, newFolder string) (string, error) {
	p := normalizeToken(product)
	c := normalizeToken(color)
	s := normalizeToken(section)
	n := normalizeToken(newFolder)

	if n == "" {
		return "", errors.New("new folder name is required")
	}

	parentPath := f.root
	switch level {
	case LevelProduct:
		// Root level: create product folder directly under root.
	case LevelColor:
		if p == "" {
			return "", errors.New("product is required for color level")
		}
		if resolved, err := f.resolveExistingName(f.root, p); err != nil {
			return "", err
		} else if resolved != "" {
			p = resolved
		}
		parentPath = joinDisk(f.root, p)
	case LevelSection:
		if p == "" || c == "" {
			return "", errors.New("product and color are required for section level")
		}
		if resolved, err := f.resolveExistingName(f.root, p); err != nil {
			return "", err
		} else if resolved != "" {
			p = resolved
		}
		resolvedColor, err := f.resolveColorFolder(joinDisk(f.root, p), p, c)
		if err != nil {
			return "", err
		}
		if resolvedColor != "" {
			c = resolvedColor
		} else {
			c = BuildColorFolder(p, c)
		}
		parentPath = joinDisk(f.root, p, c)
	default:
		return "", errors.New("unknown level")
	}

	if level == LevelSection && s != "" {
		if resolved, err := f.resolveExistingName(parentPath, s); err != nil {
			return "", err
		} else if resolved != "" {
			s = resolved
		}
		parentPath = joinDisk(parentPath, s)
	}

	target := joinDisk(parentPath, n)
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
	return strings.TrimSpace(v)
}

func (f *Flow) resolveExistingName(parentPath, desired string) (string, error) {
	desired = canonicalSectionName(normalizeToken(desired))
	if desired == "" {
		return "", nil
	}

	items, err := f.listCached(parentPath)
	if err != nil {
		return "", err
	}
	for _, name := range items {
		if strings.EqualFold(name, desired) {
			return name, nil
		}
	}
	return bestMatch(items, desired), nil
}

func (f *Flow) resolveColorFolder(parentPath, product, desiredColor string) (string, error) {
	desiredColor = normalizeToken(desiredColor)
	if desiredColor == "" {
		return "", nil
	}

	items, err := f.listCached(parentPath)
	if err != nil {
		return "", err
	}

	for _, name := range items {
		if strings.EqualFold(name, desiredColor) {
			return name, nil
		}
	}

	prefixed := BuildColorFolder(product, desiredColor)
	for _, name := range items {
		if strings.EqualFold(name, prefixed) {
			return name, nil
		}
	}

	return "", nil
}

func (f *Flow) listCached(path string) ([]string, error) {
	if f.cache != nil {
		if values, ok := f.cache.Get(path); ok {
			observability.CacheHit()
			return values, nil
		}
		observability.CacheMiss()
	}
	values, err := f.disk.ListSubdirs(path)
	if err != nil {
		return nil, err
	}
	if f.cache != nil {
		f.cache.Set(path, values)
	}
	return values, nil
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
