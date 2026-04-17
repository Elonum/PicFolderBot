package telegram

import (
	"errors"
	"testing"
)

func TestHumanErrorTranslatesServiceValidation(t *testing.T) {
	got := humanError(errors.New("product is required for color level"))
	if got == "" || got == "product is required for color level" {
		t.Fatalf("expected translated message, got: %q", got)
	}
}

func TestHumanErrorKeepsUsefulFallback(t *testing.T) {
	got := humanError(errors.New("some unexpected error"))
	if got != "some unexpected error" {
		t.Fatalf("expected passthrough, got: %q", got)
	}
}
