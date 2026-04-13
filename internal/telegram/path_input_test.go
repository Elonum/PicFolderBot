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
