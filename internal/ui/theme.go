package ui

import "github.com/charmbracelet/lipgloss"

// GitHub Dark color palette.
var (
	ColorBg        = lipgloss.Color("#0d1117")
	ColorBgDarker  = lipgloss.Color("#161b22")
	ColorFg        = lipgloss.Color("#e6edf3")
	ColorSecondary = lipgloss.Color("#8b949e")
	ColorPrimary   = lipgloss.Color("#58a6ff")
	ColorSuccess   = lipgloss.Color("#3fb950")
	ColorError     = lipgloss.Color("#f85149")
	ColorWarning   = lipgloss.Color("#d29922")
	ColorBorder    = lipgloss.Color("#21262d")
	ColorBorderLt  = lipgloss.Color("#30363d")
	ColorDimText   = lipgloss.Color("#484f58")
	ColorHighlight = lipgloss.Color("#1f6feb")
)

// Reusable styles.
var (
	StyleApp = lipgloss.NewStyle().
			Background(ColorBg).
			Foreground(ColorFg)

	StyleNavPanel = lipgloss.NewStyle().
			Width(22).
			BorderRight(true).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(ColorBorder).
			Padding(1, 1)

	StyleNavLabel = lipgloss.NewStyle().
			Foreground(ColorSecondary).
			Bold(true)

	StyleNavBtn = lipgloss.NewStyle().
			Foreground(ColorFg).
			PaddingLeft(2)

	StyleNavBtnActive = lipgloss.NewStyle().
				Foreground(ColorPrimary).
				Bold(true).
				PaddingLeft(1).
				SetString("> ")

	StyleListHeader = lipgloss.NewStyle().
			Foreground(ColorFg).
			Bold(true).
			PaddingBottom(1)

	StyleDetailPanel = lipgloss.NewStyle().
				Width(34).
				BorderLeft(true).
				BorderStyle(lipgloss.NormalBorder()).
				BorderForeground(ColorBorder).
				Padding(1, 1)

	StyleDetailTitle = lipgloss.NewStyle().
				Foreground(ColorFg).
				Bold(true).
				MarginBottom(1)

	StyleDetailLabel = lipgloss.NewStyle().
				Foreground(ColorDimText).
				Bold(true).
				Width(12)

	StyleDetailValue = lipgloss.NewStyle().
				Foreground(ColorSecondary)

	StyleStatusBar = lipgloss.NewStyle().
			Background(ColorBgDarker).
			Foreground(ColorSecondary).
			Height(1).
			Padding(0, 1)

	StyleStatusViewName = lipgloss.NewStyle().
				Foreground(ColorPrimary).
				Bold(true)

	StyleStatusHints = lipgloss.NewStyle().
				Foreground(ColorDimText)

	StyleSearchBar = lipgloss.NewStyle().
			BorderBottom(true).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(ColorBorder).
			Padding(0, 1).
			Height(3)

	StyleSearchPrefix = lipgloss.NewStyle().
				Foreground(ColorPrimary).
				Bold(true)

	StyleModalOverlay = lipgloss.NewStyle().
				Background(ColorBg).
				BorderStyle(lipgloss.RoundedBorder()).
				BorderForeground(ColorBorderLt).
				Padding(1, 2)

	StyleSelected = lipgloss.NewStyle().
			Foreground(ColorSuccess)

	StyleExplicit = lipgloss.NewStyle().
			Foreground(ColorError).
			Bold(true)

	StyleProgressFilled = lipgloss.NewStyle().
				Foreground(ColorPrimary)

	StyleProgressEmpty = lipgloss.NewStyle().
				Foreground(ColorBorderLt)
)

// StatusColor returns the color for a download status.
func StatusColor(status string) lipgloss.Color {
	switch status {
	case "pending", "Queued":
		return ColorSecondary
	case "processing", "downloading":
		return ColorPrimary
	case "completed":
		return ColorSuccess
	case "failed":
		return ColorError
	case "partial":
		return ColorWarning
	default:
		return ColorSecondary
	}
}

// ProgressBar renders a text-based progress bar.
func ProgressBar(width int, percent float64) string {
	filled := int(float64(width) * percent / 100)
	if filled > width {
		filled = width
	}
	empty := width - filled
	bar := StyleProgressFilled.Render(repeat("=", filled)) +
		StyleProgressEmpty.Render(repeat(".", empty))
	return "[" + bar + "]"
}

func repeat(s string, n int) string {
	if n <= 0 {
		return ""
	}
	result := ""
	for i := 0; i < n; i++ {
		result += s
	}
	return result
}
