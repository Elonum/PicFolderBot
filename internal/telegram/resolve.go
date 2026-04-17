package telegram

import (
	"sort"
	"strings"
)

func resolveTypedOption(options []string, input string) string {
	return resolveTypedOptionSmart(options, input).Value
}

type optionResolution struct {
	Value       string
	Suggestions []string
}

type optionScore struct {
	value string
	score float64
}

func resolveTypedOptionSmart(options []string, input string) optionResolution {
	input = strings.TrimSpace(input)
	if input == "" {
		return optionResolution{}
	}
	normInput := normalizeLookup(input)
	if normInput == "" {
		return optionResolution{}
	}

	// 1) Strict match only.
	for _, opt := range options {
		if strings.EqualFold(strings.TrimSpace(opt), input) || normalizeLookup(opt) == normInput {
			return optionResolution{Value: opt}
		}
	}

	// 2) Safe fuzzy match with deterministic ranking.
	matches := make([]optionScore, 0, len(options))
	suggestions := make([]optionScore, 0, len(options))
	for _, opt := range options {
		normOpt := normalizeLookup(opt)
		score := fuzzyScore(normOpt, normInput)
		if score >= 0.45 {
			suggestions = append(suggestions, optionScore{value: opt, score: score})
		}
		if score >= 0.72 {
			matches = append(matches, optionScore{value: opt, score: score})
		}
	}
	if len(matches) == 0 {
		return optionResolution{Suggestions: topSuggestions(suggestions, 3)}
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].score > matches[j].score })
	best := matches[0]
	// Protect user from accidental wrong navigation:
	// if two options are too close in score, we ask user to choose manually.
	if len(matches) > 1 && best.score-matches[1].score < 0.08 {
		return optionResolution{Suggestions: topSuggestions(matches, 3)}
	}
	return optionResolution{Value: best.value}
}

func normalizeLookup(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	v = strings.ReplaceAll(v, "ё", "е")
	v = strings.ReplaceAll(v, "-", " ")
	v = strings.ReplaceAll(v, "_", " ")
	// Map Cyrillic/Latin confusables to a unified Latin form.
	v = strings.Map(func(r rune) rune {
		switch r {
		// Cyrillic -> Latin
		case 'а':
			return 'a'
		case 'в':
			return 'b'
		case 'с':
			return 'c'
		case 'е':
			return 'e'
		case 'н':
			return 'h'
		case 'к':
			return 'k'
		case 'м':
			return 'm'
		case 'о':
			return 'o'
		case 'р':
			return 'p'
		case 'т':
			return 't'
		case 'х':
			return 'x'
		case 'у':
			return 'y'
		default:
			return r
		}
	}, v)
	return strings.Join(strings.Fields(v), " ")
}

func fuzzyScore(option, input string) float64 {
	if option == "" || input == "" {
		return 0
	}
	if option == input {
		return 1
	}

	// Prefix match is useful for short article-style names.
	prefix := 0.0
	if strings.HasPrefix(option, input) || strings.HasPrefix(input, option) {
		// Penalize large length mismatch (prevents F05P02 -> F05P0200 mistakes).
		diff := absInt(len(option) - len(input))
		prefix = 0.9 - float64(diff)*0.06
		if prefix < 0 {
			prefix = 0
		}
	}

	// Token overlap score.
	optionTokens := strings.Fields(option)
	inputTokens := strings.Fields(input)
	overlap := tokenOverlap(optionTokens, inputTokens)

	if overlap > prefix {
		return overlap
	}
	return prefix
}

func tokenOverlap(a, b []string) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	setA := make(map[string]struct{}, len(a))
	for _, v := range a {
		setA[v] = struct{}{}
	}
	common := 0
	for _, v := range b {
		if _, ok := setA[v]; ok {
			common++
		}
	}
	if common == 0 {
		return 0
	}
	denominator := len(a)
	if len(b) > denominator {
		denominator = len(b)
	}
	return float64(common) / float64(denominator)
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func topSuggestions(values []optionScore, limit int) []string {
	if len(values) == 0 || limit <= 0 {
		return nil
	}
	sort.Slice(values, func(i, j int) bool { return values[i].score > values[j].score })
	seen := make(map[string]struct{}, limit)
	out := make([]string, 0, limit)
	for _, v := range values {
		if _, ok := seen[v.value]; ok {
			continue
		}
		seen[v.value] = struct{}{}
		out = append(out, v.value)
		if len(out) == limit {
			break
		}
	}
	return out
}
