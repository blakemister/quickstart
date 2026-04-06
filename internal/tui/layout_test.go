package tui

import "testing"

func TestLayoutForCount(t *testing.T) {
	tests := []struct {
		count int
		want  string
	}{
		{0, "full"},
		{1, "full"},
		{2, "vertical"},
		{3, "grid"},
		{4, "grid"},
		{9, "grid"},
	}

	for _, tt := range tests {
		got := LayoutForCount(tt.count)
		if got != tt.want {
			t.Errorf("LayoutForCount(%d) = %q, want %q", tt.count, got, tt.want)
		}
	}
}
