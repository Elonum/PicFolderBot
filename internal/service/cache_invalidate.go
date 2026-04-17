package service

// Cache invalidation helpers used by Telegram UI "Refresh" buttons.
// They are no-ops when cache is not configured.

func (f *Flow) InvalidateProducts() {
	if f.cache == nil {
		return
	}
	f.cache.Delete(f.root)
}

func (f *Flow) InvalidateColors(product string) {
	if f.cache == nil {
		return
	}
	p := normalizeToken(product)
	if p == "" {
		f.cache.Delete(f.root)
		return
	}
	if resolved, _ := f.resolveExistingName(f.root, p); resolved != "" {
		p = resolved
	}
	f.cache.Delete(joinDisk(f.root, p))
}

func (f *Flow) InvalidateSections(product, color string) {
	if f.cache == nil {
		return
	}
	p := normalizeToken(product)
	if p == "" {
		f.cache.Delete(f.root)
		return
	}
	if resolved, _ := f.resolveExistingName(f.root, p); resolved != "" {
		p = resolved
	}
	c := normalizeToken(color)
	if c == "" {
		f.cache.Delete(joinDisk(f.root, p))
		return
	}
	// Sections are cached by exact path root/product/<color-folder>.
	// Invalidate the most probable color folder paths (direct, resolved, and prefixed).
	parent := joinDisk(f.root, p)
	resolvedColor, _ := f.resolveColorFolder(parent, p, c)
	if resolvedColor != "" {
		f.cache.Delete(joinDisk(f.root, p, resolvedColor))
	}
	f.cache.Delete(joinDisk(f.root, p, c))
	if prefixed := BuildColorFolder(p, c); prefixed != "" {
		f.cache.Delete(joinDisk(f.root, p, prefixed))
	}
}
