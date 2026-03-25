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
