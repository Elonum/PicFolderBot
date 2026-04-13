package parser

import "testing"

func TestParseCaption(t *testing.T) {
	in := "Товар=F05P01, цвет: жёлт, папка: Титульники"
	got := ParseCaption(in)

	if got.Product != "F05P01" {
		t.Fatalf("unexpected product: %s", got.Product)
	}
	if got.Color != "жёлт" {
		t.Fatalf("unexpected color: %s", got.Color)
	}
	if got.Section != "Титульники" {
		t.Fatalf("unexpected section: %s", got.Section)
	}
}
