package telegram

import "testing"

func TestParseFullPathInputSeparators(t *testing.T) {
	p, c, s, ok := parseFullPathInput("F05P01, F05P01-ЖЁЛТ, Рич-контент")
	if !ok || p != "F05P01" || c != "F05P01-ЖЁЛТ" || s != "Рич-контент" {
		t.Fatalf("unexpected parsed tuple: %v %q %q %q", ok, p, c, s)
	}
}

func TestParseFullPathInputWhitespace(t *testing.T) {
	p, c, s, ok := parseFullPathInput("F05P01 F05P01-ЖЁЛТ Рич контент")
	if !ok || p != "F05P01" || c != "F05P01-ЖЁЛТ" || s != "Рич контент" {
		t.Fatalf("unexpected parsed tuple: %v %q %q %q", ok, p, c, s)
	}
}

func TestResolveTypedOption(t *testing.T) {
	options := []string{"F05P01-ЖЁЛТ", "F05P01-КРАСН"}
	got := resolveTypedOption(options, "f05p01-жёлт")
	if got != "F05P01-ЖЁЛТ" {
		t.Fatalf("unexpected resolved option: %s", got)
	}
}

func TestResolveTypedOptionDoesNotPickLongerPrefixByMistake(t *testing.T) {
	options := []string{"F05P020", "F05P02"}
	got := resolveTypedOption(options, "F05P02")
	if got != "F05P02" {
		t.Fatalf("expected exact code match, got: %s", got)
	}
}

func TestResolveTypedOptionAmbiguousReturnsEmpty(t *testing.T) {
	options := []string{"Rich Content Main", "Rich Content Media"}
	got := resolveTypedOption(options, "rich content")
	if got != "" {
		t.Fatalf("expected ambiguous fuzzy match to be empty, got: %s", got)
	}
}

func TestResolveTypedOptionSmartAmbiguousHasSuggestions(t *testing.T) {
	options := []string{"Rich Content Main", "Rich Content Media", "Manual"}
	res := resolveTypedOptionSmart(options, "rich content")
	if res.Value != "" {
		t.Fatalf("expected empty value for ambiguous result, got: %s", res.Value)
	}
	if len(res.Suggestions) < 2 {
		t.Fatalf("expected suggestions for ambiguous match, got: %#v", res.Suggestions)
	}
}

func TestResolveTypedOptionSmartNotFoundHasClosestSuggestions(t *testing.T) {
	options := []string{"F05P01", "F05P02", "F06P01"}
	res := resolveTypedOptionSmart(options, "F05")
	if len(res.Suggestions) == 0 {
		t.Fatalf("expected at least one suggestion, got none")
	}
}

func TestNormalizeLookupConfusablesCyrToLatin(t *testing.T) {
	// Cyrillic 'р' should be treated as Latin 'p' in article codes.
	got := normalizeLookup("F06Р03") // Р is Cyrillic
	if got != "f06p03" {
		t.Fatalf("unexpected normalize: %q", got)
	}
}
