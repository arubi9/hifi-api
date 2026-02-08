package ui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mrpir/hifi-tui/internal/config"
)

// SettingsScreenModel is the settings modal.
type SettingsScreenModel struct {
	settings      config.Settings
	dirInput      textinput.Model
	qualityIndex  int
	parallelIndex int
	focusField    int // 0=dir, 1=quality, 2=parallel, 3=save, 4=cancel
	width         int
	height        int
}

var qualityOptions = []string{"HI_RES_LOSSLESS", "LOSSLESS", "HIGH", "LOW"}

// NewSettingsScreen creates a settings modal with current values.
func NewSettingsScreen(s config.Settings, w, h int) SettingsScreenModel {
	ti := textinput.New()
	ti.SetValue(s.DownloadDir)
	ti.Width = 40
	ti.Focus()

	qi := 0
	for i, q := range qualityOptions {
		if q == s.Quality {
			qi = i
			break
		}
	}

	pi := s.ParallelCount - 1
	if pi < 0 {
		pi = 0
	}
	if pi > 49 {
		pi = 49
	}

	return SettingsScreenModel{
		settings:      s,
		dirInput:      ti,
		qualityIndex:  qi,
		parallelIndex: pi,
		width:         w,
		height:        h,
	}
}

// Update handles settings screen input.
func (s *SettingsScreenModel) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "tab", "down":
			s.focusField = (s.focusField + 1) % 5
			s.updateFocus()
		case "shift+tab", "up":
			s.focusField = (s.focusField + 4) % 5
			s.updateFocus()
		case "left":
			if s.focusField == 1 {
				if s.qualityIndex > 0 {
					s.qualityIndex--
				}
			} else if s.focusField == 2 {
				if s.parallelIndex > 0 {
					s.parallelIndex--
				}
			}
		case "right":
			if s.focusField == 1 {
				if s.qualityIndex < len(qualityOptions)-1 {
					s.qualityIndex++
				}
			} else if s.focusField == 2 {
				if s.parallelIndex < 49 {
					s.parallelIndex++
				}
			}
		case "enter":
			if s.focusField == 3 {
				return s.save()
			}
			if s.focusField == 4 {
				return func() tea.Msg { return SettingsClosedMsg{} }
			}
		}
	}

	if s.focusField == 0 {
		var cmd tea.Cmd
		s.dirInput, cmd = s.dirInput.Update(msg)
		return cmd
	}
	return nil
}

func (s *SettingsScreenModel) updateFocus() {
	if s.focusField == 0 {
		s.dirInput.Focus()
	} else {
		s.dirInput.Blur()
	}
}

func (s *SettingsScreenModel) save() tea.Cmd {
	newSettings := config.Settings{
		DownloadDir:   s.dirInput.Value(),
		Quality:       qualityOptions[s.qualityIndex],
		ParallelCount: s.parallelIndex + 1,
		LastQuery:     s.settings.LastQuery,
	}
	if err := config.Save(newSettings); err != nil {
		return func() tea.Msg { return ErrorMsg{Err: err} }
	}
	return func() tea.Msg {
		return SettingsSavedMsg{Settings: newSettings}
	}
}

// SettingsClosedMsg signals the settings screen was cancelled.
type SettingsClosedMsg struct{}

// SettingsSavedMsg signals settings were saved.
type SettingsSavedMsg struct {
	Settings config.Settings
}

// View renders the settings screen.
func (s *SettingsScreenModel) View() string {
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorFg).
		MarginBottom(1).
		Render("Settings")

	labelStyle := lipgloss.NewStyle().
		Foreground(ColorSecondary).
		Bold(true).
		Width(20)

	focusIndicator := func(field int) string {
		if s.focusField == field {
			return lipgloss.NewStyle().Foreground(ColorPrimary).Render("> ")
		}
		return "  "
	}

	// Download directory
	dirRow := focusIndicator(0) + labelStyle.Render("Download Directory") + "\n" +
		"  " + s.dirInput.View()

	// Quality
	qualityStr := ""
	for i, q := range qualityOptions {
		style := lipgloss.NewStyle().Foreground(ColorSecondary)
		if i == s.qualityIndex {
			style = lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true)
		}
		qShort := shortQuality(q)
		qualityStr += style.Render(qShort) + "  "
	}
	qualityRow := focusIndicator(1) + labelStyle.Render("Audio Quality") + qualityStr

	// Parallel count
	pc := s.parallelIndex + 1
	parallelRow := focusIndicator(2) + labelStyle.Render("Parallel Downloads") +
		lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true).Render(fmt.Sprintf("< %d >", pc))

	// Buttons
	saveStyle := lipgloss.NewStyle().Foreground(ColorSuccess).Bold(true)
	cancelStyle := lipgloss.NewStyle().Foreground(ColorSecondary)
	if s.focusField == 3 {
		saveStyle = saveStyle.Background(ColorHighlight)
	}
	if s.focusField == 4 {
		cancelStyle = cancelStyle.Background(ColorHighlight)
	}
	buttons := "  " + saveStyle.Render(" Save ") + "  " + cancelStyle.Render(" Cancel ")

	content := title + "\n\n" +
		dirRow + "\n\n" +
		qualityRow + "\n\n" +
		parallelRow + "\n\n" +
		buttons + "\n\n" +
		lipgloss.NewStyle().Foreground(ColorSecondary).Render("Tab/Shift+Tab to navigate, Left/Right to change, Enter to select")

	boxWidth := 64
	if boxWidth > s.width*90/100 {
		boxWidth = s.width * 90 / 100
	}

	return StyleModalOverlay.
		Width(boxWidth).
		Render(content)
}
