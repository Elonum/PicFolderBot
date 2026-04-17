package telegram

import "testing"

func TestResolveBackTargetFallbacks(t *testing.T) {
	b := &Bot{}
	state := &sessionState{}

	if got := b.resolveBackTarget("section", state); got != "product" {
		t.Fatalf("expected product fallback, got %s", got)
	}
	state.Product = "F01"
	if got := b.resolveBackTarget("section", state); got != "color" {
		t.Fatalf("expected color fallback, got %s", got)
	}
	state.Color = "F01-RED"
	if got := b.resolveBackTarget("section", state); got != "section" {
		t.Fatalf("expected section, got %s", got)
	}
}
