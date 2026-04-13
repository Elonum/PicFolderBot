package parser

import (
	"regexp"
	"strings"
)

type ParsedInput struct {
	Product string
	Color   string
	Section string
}

var (
	productRe = regexp.MustCompile(`(?i)\b[F]\d{2}[P]\d{2}\b`)
	colorRe   = regexp.MustCompile(`(?i)(?:цвет|color)\s*[:=]\s*([^\n,;]+)`)
	sectionRe = regexp.MustCompile(`(?i)(?:папка|раздел|folder|section)\s*[:=]\s*([^\n,;]+)`)
)

func ParseCaption(caption string) ParsedInput {
	caption = strings.TrimSpace(caption)
	if caption == "" {
		return ParsedInput{}
	}

	out := ParsedInput{}
	if m := productRe.FindString(caption); m != "" {
		out.Product = strings.ToUpper(strings.TrimSpace(m))
	}
	if m := colorRe.FindStringSubmatch(caption); len(m) > 1 {
		out.Color = normalizeText(m[1])
	}
	if m := sectionRe.FindStringSubmatch(caption); len(m) > 1 {
		out.Section = normalizeText(m[1])
	}

	return out
}

func normalizeText(v string) string {
	return strings.TrimSpace(v)
}
