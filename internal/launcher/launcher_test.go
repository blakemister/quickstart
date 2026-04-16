package launcher

import (
	"testing"

	"github.com/bcmister/qs/internal/monitor"
)

func TestCalculateLayoutFull(t *testing.T) {
	mon := &monitor.Monitor{X: 0, Y: 0, Width: 1920, Height: 1080}
	positions := CalculateLayout(mon, 1, "full")

	if len(positions) != 1 {
		t.Fatalf("expected 1 position, got %d", len(positions))
	}

	p := positions[0]
	if p.X != 0 || p.Y != 0 || p.Width != 1920 || p.Height != 1080 {
		t.Errorf("expected full screen (0,0,1920,1080), got (%d,%d,%d,%d)", p.X, p.Y, p.Width, p.Height)
	}
}

func TestCalculateLayoutVertical(t *testing.T) {
	mon := &monitor.Monitor{X: 0, Y: 0, Width: 1920, Height: 1080}
	positions := CalculateLayout(mon, 2, "vertical")

	if len(positions) != 2 {
		t.Fatalf("expected 2 positions, got %d", len(positions))
	}

	// Two side-by-side columns
	if positions[0].X != 0 || positions[0].Width != 960 {
		t.Errorf("pos 0: expected X=0 Width=960, got X=%d Width=%d", positions[0].X, positions[0].Width)
	}
	if positions[1].X != 960 || positions[1].Width != 960 {
		t.Errorf("pos 1: expected X=960 Width=960, got X=%d Width=%d", positions[1].X, positions[1].Width)
	}
	// Both should be full height
	for i, p := range positions {
		if p.Height != 1080 {
			t.Errorf("pos %d: expected Height=1080, got %d", i, p.Height)
		}
	}
}

func TestCalculateLayoutHorizontal(t *testing.T) {
	mon := &monitor.Monitor{X: 0, Y: 0, Width: 1920, Height: 1080}
	positions := CalculateLayout(mon, 2, "horizontal")

	if len(positions) != 2 {
		t.Fatalf("expected 2 positions, got %d", len(positions))
	}

	// Two stacked rows
	if positions[0].Y != 0 || positions[0].Height != 540 {
		t.Errorf("pos 0: expected Y=0 Height=540, got Y=%d Height=%d", positions[0].Y, positions[0].Height)
	}
	if positions[1].Y != 540 || positions[1].Height != 540 {
		t.Errorf("pos 1: expected Y=540 Height=540, got Y=%d Height=%d", positions[1].Y, positions[1].Height)
	}
	// Both should be full width
	for i, p := range positions {
		if p.Width != 1920 {
			t.Errorf("pos %d: expected Width=1920, got %d", i, p.Width)
		}
	}
}

func TestCalculateLayoutGrid(t *testing.T) {
	mon := &monitor.Monitor{X: 0, Y: 0, Width: 1920, Height: 1080}
	positions := CalculateLayout(mon, 4, "grid")

	if len(positions) != 4 {
		t.Fatalf("expected 4 positions, got %d", len(positions))
	}

	// 2x2 grid
	expected := []Position{
		{X: 0, Y: 0, Width: 960, Height: 540},
		{X: 960, Y: 0, Width: 960, Height: 540},
		{X: 0, Y: 540, Width: 960, Height: 540},
		{X: 960, Y: 540, Width: 960, Height: 540},
	}

	for i, exp := range expected {
		if positions[i] != exp {
			t.Errorf("pos %d: expected %+v, got %+v", i, exp, positions[i])
		}
	}
}

func TestCalculateLayoutGridOdd(t *testing.T) {
	mon := &monitor.Monitor{X: 0, Y: 0, Width: 1920, Height: 1080}
	positions := CalculateLayout(mon, 3, "grid")

	if len(positions) != 3 {
		t.Fatalf("expected 3 positions, got %d", len(positions))
	}

	// 2 cols x 2 rows grid, 3 cells filled
	// Cell dimensions: 960 x 540
	if positions[0].Width != 960 || positions[0].Height != 540 {
		t.Errorf("pos 0: expected 960x540, got %dx%d", positions[0].Width, positions[0].Height)
	}
	if positions[1].X != 960 {
		t.Errorf("pos 1: expected X=960, got X=%d", positions[1].X)
	}
	if positions[2].Y != 540 {
		t.Errorf("pos 2: expected Y=540, got Y=%d", positions[2].Y)
	}
}

func TestCalculateLayoutWithOffset(t *testing.T) {
	// Monitor at offset position
	mon := &monitor.Monitor{X: 1920, Y: 0, Width: 1920, Height: 1080}
	positions := CalculateLayout(mon, 2, "vertical")

	if positions[0].X != 1920 {
		t.Errorf("pos 0: expected X=1920, got X=%d", positions[0].X)
	}
	if positions[1].X != 2880 {
		t.Errorf("pos 1: expected X=2880, got X=%d", positions[1].X)
	}
}

// TestEdgeSnapping verifies that cells tile to the exact monitor boundary
// even when dimensions are not evenly divisible.
func TestEdgeSnapping(t *testing.T) {
	tests := []struct {
		name   string
		mon    monitor.Monitor
		count  int
		layout string
	}{
		{"vertical 3 on 2560", monitor.Monitor{X: 0, Y: 0, Width: 2560, Height: 1440}, 3, "vertical"},
		{"horizontal 3 on 1080", monitor.Monitor{X: 0, Y: 0, Width: 1920, Height: 1080}, 3, "horizontal"},
		{"grid 5 on 2560x1440", monitor.Monitor{X: 0, Y: 0, Width: 2560, Height: 1440}, 5, "grid"},
		{"grid 3 on 2560x1440", monitor.Monitor{X: 0, Y: 0, Width: 2560, Height: 1440}, 3, "grid"},
		{"vertical 3 on offset monitor", monitor.Monitor{X: 2560, Y: 0, Width: 2560, Height: 1440}, 3, "vertical"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			positions := CalculateLayout(&tt.mon, tt.count, tt.layout)

			monRight := tt.mon.X + tt.mon.Width
			monBottom := tt.mon.Y + tt.mon.Height

			// Track the rightmost and bottommost edges across all cells
			maxRight := 0
			maxBottom := 0

			for _, p := range positions {
				right := p.X + p.Width
				bottom := p.Y + p.Height

				if right > monRight {
					t.Errorf("cell %+v exceeds monitor right edge %d", p, monRight)
				}
				if bottom > monBottom {
					t.Errorf("cell %+v exceeds monitor bottom edge %d", p, monBottom)
				}
				if right > maxRight {
					maxRight = right
				}
				if bottom > maxBottom {
					maxBottom = bottom
				}
			}

			// Some cell must reach the right edge, and some cell must reach the bottom
			if maxRight != monRight {
				t.Errorf("rightmost cell edge %d != monitor right %d (gap = %d)",
					maxRight, monRight, monRight-maxRight)
			}
			if maxBottom != monBottom {
				t.Errorf("bottommost cell edge %d != monitor bottom %d (gap = %d)",
					maxBottom, monBottom, monBottom-maxBottom)
			}
		})
	}
}

// TestCalculateLayoutZero verifies zero-count requests return an empty slice
// (used by the skip-monitor path in `qs all`).
func TestCalculateLayoutZero(t *testing.T) {
	mon := &monitor.Monitor{X: 0, Y: 0, Width: 1920, Height: 1080}
	for _, layout := range []string{"grid", "vertical", "horizontal"} {
		t.Run(layout, func(t *testing.T) {
			positions := CalculateLayout(mon, 0, layout)
			if len(positions) != 0 {
				t.Errorf("expected 0 positions for count=0, got %d", len(positions))
			}
		})
	}
}

// TestCalculateLayoutUnknownFallsBackToGrid verifies unknown layout strings
// are handled as grid rather than panicking.
func TestCalculateLayoutUnknownFallsBackToGrid(t *testing.T) {
	mon := &monitor.Monitor{X: 0, Y: 0, Width: 1920, Height: 1080}
	got := CalculateLayout(mon, 4, "not-a-layout")
	want := CalculateLayout(mon, 4, "grid")
	if len(got) != len(want) {
		t.Fatalf("unknown layout produced %d positions, want %d", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("pos %d: got %+v, want %+v", i, got[i], want[i])
		}
	}
}

// TestVerticalEdgeSnappingAllCells verifies every cell boundary is contiguous.
func TestVerticalEdgeSnappingAllCells(t *testing.T) {
	mon := &monitor.Monitor{X: 0, Y: 0, Width: 2560, Height: 1440}
	positions := CalculateLayout(mon, 3, "vertical")

	// Each cell's right edge should be the next cell's left edge
	for i := 0; i < len(positions)-1; i++ {
		right := positions[i].X + positions[i].Width
		nextLeft := positions[i+1].X
		if right != nextLeft {
			t.Errorf("gap between cell %d (right=%d) and cell %d (left=%d): %d px",
				i, right, i+1, nextLeft, nextLeft-right)
		}
	}

	// First cell starts at monitor left
	if positions[0].X != mon.X {
		t.Errorf("first cell X=%d, expected %d", positions[0].X, mon.X)
	}

	// Last cell ends at monitor right
	last := positions[len(positions)-1]
	if last.X+last.Width != mon.X+mon.Width {
		t.Errorf("last cell right=%d, expected %d", last.X+last.Width, mon.X+mon.Width)
	}
}
