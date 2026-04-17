package telegram

import "testing"

func TestCloneSessionStateDeepCopiesMutableFields(t *testing.T) {
	src := &sessionState{
		FileBytes: []byte{1, 2, 3},
		ValueMap:  map[string]string{"k": "v"},
	}
	cp := cloneSessionState(src)
	cp.FileBytes[0] = 9
	cp.ValueMap["k"] = "x"

	if src.FileBytes[0] != 1 {
		t.Fatalf("expected FileBytes to be deep copied")
	}
	if src.ValueMap["k"] != "v" {
		t.Fatalf("expected ValueMap to be deep copied")
	}
}
