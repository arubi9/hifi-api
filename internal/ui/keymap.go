package ui

import "github.com/charmbracelet/bubbles/key"

// KeyMap holds all key bindings.
type KeyMap struct {
	Search         key.Binding
	Help           key.Binding
	Quit           key.Binding
	Download       key.Binding
	ToggleSelect   key.Binding
	SelectAll      key.Binding
	ClearSelection key.Binding
	Back           key.Binding
	Menu           key.Binding
	Settings       key.Binding
	CancelJob      key.Binding
	Enter          key.Binding
	Up             key.Binding
	Down           key.Binding
	Tab            key.Binding
	ShiftTab       key.Binding
	Escape         key.Binding
}

// DefaultKeyMap returns the default key bindings (matching Python TUI).
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Search: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "Search"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "Help"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "Quit"),
		),
		Download: key.NewBinding(
			key.WithKeys("d"),
			key.WithHelp("d", "Download"),
		),
		ToggleSelect: key.NewBinding(
			key.WithKeys(" "),
			key.WithHelp("space", "Select"),
		),
		SelectAll: key.NewBinding(
			key.WithKeys("a"),
			key.WithHelp("a", "Select all"),
		),
		ClearSelection: key.NewBinding(
			key.WithKeys("A"),
			key.WithHelp("A", "Clear selection"),
		),
		Back: key.NewBinding(
			key.WithKeys("backspace"),
			key.WithHelp("backspace", "Back"),
		),
		Menu: key.NewBinding(
			key.WithKeys("m"),
			key.WithHelp("m", "Menu"),
		),
		Settings: key.NewBinding(
			key.WithKeys("s"),
			key.WithHelp("s", "Settings"),
		),
		CancelJob: key.NewBinding(
			key.WithKeys("c"),
			key.WithHelp("c", "Cancel"),
		),
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "Open"),
		),
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("up/k", "Up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("down/j", "Down"),
		),
		Tab: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "Next panel"),
		),
		ShiftTab: key.NewBinding(
			key.WithKeys("shift+tab"),
			key.WithHelp("shift+tab", "Prev panel"),
		),
		Escape: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "Close"),
		),
	}
}
