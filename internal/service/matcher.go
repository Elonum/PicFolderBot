package service

import "strings"

var sectionAliases = map[string]string{
	"титул":        "Титульники",
	"титульник":    "Титульники",
	"титульники":   "Титульники",
	"title":        "Титульники",
	"rich":         "Рич-Контент",
	"rich content": "Рич-Контент",
	"рк":           "Рич-Контент",
	"рич":          "Рич-Контент",
	"рич контент":  "Рич-Контент",
	"рич-контент":  "Рич-Контент",
}

func normalizeSearchToken(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	v = strings.ReplaceAll(v, "ё", "е")
	v = strings.ReplaceAll(v, "_", " ")
	v = strings.ReplaceAll(v, "-", " ")
	v = strings.Join(strings.Fields(v), " ")
	return v
}

func canonicalSectionName(v string) string {
	if canonical, ok := sectionAliases[normalizeSearchToken(v)]; ok {
		return canonical
	}
	return strings.TrimSpace(v)
}

func bestMatch(options []string, input string) string {
	input = canonicalSectionName(input)
	normInput := normalizeSearchToken(input)
	if normInput == "" {
		return ""
	}
	best := ""
	bestScore := -1
	for _, option := range options {
		normOption := normalizeSearchToken(option)
		score := 0
		switch {
		case normOption == normInput:
			score = 100
		case strings.Contains(normOption, normInput) || strings.Contains(normInput, normOption):
			score = 70
		default:
			score = overlapScore(normOption, normInput)
		}
		if score > bestScore {
			bestScore = score
			best = option
		}
	}
	if bestScore < 55 {
		return ""
	}
	return best
}

func overlapScore(a, b string) int {
	if a == "" || b == "" {
		return 0
	}
	score := 0
	for _, token := range strings.Fields(b) {
		if strings.Contains(a, token) {
			score += 20
		}
	}
	return score
}
