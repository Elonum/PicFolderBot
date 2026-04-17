package telegram

import "testing"

func TestIsTitularSectionName(t *testing.T) {
	cases := []string{
		"Титульники",
		"титульник",
		"ТИТУЛЬНИКИ",
		"Титульник (главное)",
		"title",
	}
	for _, c := range cases {
		if !isTitularSectionName(c) {
			t.Fatalf("expected titular: %q", c)
		}
	}
	if isTitularSectionName("Рич-Контент") {
		t.Fatalf("did not expect titular for rich content")
	}
}

func TestApplyRenameInputSuffix(t *testing.T) {
	got := applyRenameInput("img_1.jpg", "Иванов", "image/jpeg")
	if got != "Иванов.jpg" {
		t.Fatalf("expected renamed filename")
	}
}

func TestApplyRenameInputFullName(t *testing.T) {
	got := applyRenameInput("img_1.jpg", "F06P03_Иванов.png", "image/png")
	if got != "F06P03_Иванов.png" {
		t.Fatalf("unexpected full rename: %q", got)
	}
}
