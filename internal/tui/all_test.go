package tui

import (
	"testing"

	"github.com/bcmister/qs/internal/config"
	"github.com/bcmister/qs/internal/monitor"
	tea "github.com/charmbracelet/bubbletea"
)

func newTestAllModel(counts []int) AllModel {
	m := NewAll(&config.Config{})
	m.monitors = make([]monitor.Monitor, len(counts))
	m.windowCounts = append([]int{}, counts...)
	return m
}

func TestAllModel_EnterBlockedWhenAllCountsZero(t *testing.T) {
	m := newTestAllModel([]int{0, 0, 0})
	updated, _ := m.updateMonitors(tea.KeyMsg{Type: tea.KeyEnter})
	result := updated.(AllModel)
	if result.Confirmed() {
		t.Error("Enter should not confirm when total windows is 0")
	}
	if result.Cancelled() {
		t.Error("Enter should not quit the model; user should be able to raise a count and retry")
	}
}

func TestAllModel_EnterConfirmsWhenAnyCountNonzero(t *testing.T) {
	m := newTestAllModel([]int{0, 1, 0})
	updated, _ := m.updateMonitors(tea.KeyMsg{Type: tea.KeyEnter})
	result := updated.(AllModel)
	if !result.Confirmed() {
		t.Error("Enter should confirm when at least one monitor has a non-zero count")
	}
}

func TestAllModel_DownArrowCanReachZero(t *testing.T) {
	m := newTestAllModel([]int{2})
	for i := 0; i < 3; i++ {
		updated, _ := m.updateMonitors(tea.KeyMsg{Type: tea.KeyDown})
		m = updated.(AllModel)
	}
	if m.windowCounts[0] != 0 {
		t.Errorf("expected count to clamp at 0, got %d", m.windowCounts[0])
	}
}

func TestAllModel_totalWindows(t *testing.T) {
	cases := []struct {
		counts []int
		want   int
	}{
		{[]int{}, 0},
		{[]int{0, 0}, 0},
		{[]int{1, 2, 3}, 6},
		{[]int{0, 5, 0}, 5},
	}
	for _, tc := range cases {
		m := newTestAllModel(tc.counts)
		if got := m.totalWindows(); got != tc.want {
			t.Errorf("counts=%v: totalWindows=%d, want %d", tc.counts, got, tc.want)
		}
	}
}
