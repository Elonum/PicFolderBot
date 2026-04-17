package telegram

import (
	"testing"

	"PicFolderBot/internal/service"
)

func TestNextAwaitingForLevel(t *testing.T) {
	tests := []struct {
		name  string
		level string
		state sessionState
		want  string
	}{
		{
			name:  "missing product always asks product",
			level: service.LevelSection,
			state: sessionState{},
			want:  "product",
		},
		{
			name:  "product level with product selected asks photo",
			level: service.LevelProduct,
			state: sessionState{Product: "F05P01"},
			want:  "photo",
		},
		{
			name:  "color level without color asks color",
			level: service.LevelColor,
			state: sessionState{Product: "F05P01"},
			want:  "color",
		},
		{
			name:  "section level without section asks section",
			level: service.LevelSection,
			state: sessionState{Product: "F05P01", Color: "F05P01-ЖЕЛТ"},
			want:  "section",
		},
		{
			name:  "section level fully selected asks photo",
			level: service.LevelSection,
			state: sessionState{Product: "F05P01", Color: "F05P01-ЖЕЛТ", Section: "Титульники"},
			want:  "photo",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := nextAwaitingForLevel(tc.level, &tc.state)
			if got != tc.want {
				t.Fatalf("unexpected awaiting: got %s want %s", got, tc.want)
			}
		})
	}
}
