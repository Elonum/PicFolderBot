package telegram

import "testing"

func TestPaginate(t *testing.T) {
	values := []string{"1", "2", "3", "4", "5"}

	page0, prev0, next0, idx0 := paginate(values, 0, 2)
	if len(page0) != 2 || page0[0] != "1" || page0[1] != "2" || prev0 || !next0 || idx0 != 0 {
		t.Fatalf("unexpected page0: %#v %v %v %d", page0, prev0, next0, idx0)
	}

	page2, prev2, next2, idx2 := paginate(values, 2, 2)
	if len(page2) != 1 || page2[0] != "5" || !prev2 || next2 || idx2 != 2 {
		t.Fatalf("unexpected page2: %#v %v %v %d", page2, prev2, next2, idx2)
	}
}

func TestStepPage(t *testing.T) {
	if got := stepPage(0, "prev"); got != 0 {
		t.Fatalf("expected 0, got %d", got)
	}
	if got := stepPage(1, "prev"); got != 0 {
		t.Fatalf("expected 0, got %d", got)
	}
	if got := stepPage(1, "next"); got != 2 {
		t.Fatalf("expected 2, got %d", got)
	}
}
