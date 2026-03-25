package tui

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

// Color palette
var (
	ColorCyan    = lipgloss.Color("86")
	ColorBrCyan  = lipgloss.Color("14")
	ColorGreen   = lipgloss.Color("10")
	ColorYellow  = lipgloss.Color("11")
	ColorRed     = lipgloss.Color("9")
	ColorWhite   = lipgloss.Color("15")
	ColorDimGray = lipgloss.Color("8")
)

// Styles
var (
	TitleStyle = lipgloss.NewStyle().
			Foreground(ColorBrCyan).
			Bold(true)

	SubtitleStyle = lipgloss.NewStyle().
			Foreground(ColorWhite).
			Bold(true)

	SelectedStyle = lipgloss.NewStyle().
			Foreground(ColorBrCyan).
			Bold(true)

	NormalStyle = lipgloss.NewStyle().
			Foreground(ColorDimGray)

	SuccessStyle = lipgloss.NewStyle().
			Foreground(ColorGreen)

	WarningStyle = lipgloss.NewStyle().
			Foreground(ColorYellow)

	ErrorStyle = lipgloss.NewStyle().
			Foreground(ColorRed)

	DimStyle = lipgloss.NewStyle().
			Foreground(ColorDimGray)

	WhiteStyle = lipgloss.NewStyle().
			Foreground(ColorWhite)

	BorderStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorDimGray).
			Padding(0, 1)
)

var logoLines = []struct {
	color lipgloss.Color
	text  string
}{
	{ColorBrCyan, "  ██████╗ ██╗  ██╗"},
	{ColorBrCyan, " ██╔═══██╗██║ ██╔╝"},
	{ColorCyan, " ██║   ██║█████╔╝ "},
	{ColorCyan, " ██║▄▄ ██║██╔═██╗ "},
	{ColorDimGray, " ╚██████╔╝██║  ██╗"},
	{ColorDimGray, "  ╚══▀▀═╝ ╚═╝  ╚═╝"},
}

// RenderLogo returns the ASCII art logo with an optional subtitle
func RenderLogo(subtitle string) string {
	var s string
	s += "\n"
	for i, l := range logoLines {
		style := lipgloss.NewStyle().Foreground(l.color)
		s += " " + style.Render(l.text)
		if i == 1 && subtitle != "" {
			s += "   " + SubtitleStyle.Render(subtitle)
		}
		s += "\n"
	}
	return s
}

// RenderSep returns a dim horizontal rule
func RenderSep() string {
	sep := ""
	for i := 0; i < 40; i++ {
		sep += "─"
	}
	return fmt.Sprintf("\n %s\n", DimStyle.Render(sep))
}
