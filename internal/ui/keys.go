package ui

import "charm.land/bubbles/v2/key"

type keyMap struct {
	Up, Down, Left, Right key.Binding
	NextPane, PrevPane    key.Binding
	Enter, Back           key.Binding
	Top, Bottom           key.Binding
	MarkRead, MarkAll     key.Binding
	Undo                  key.Binding
	Help, Quit            key.Binding
}

func defaultKeys() keyMap {
	return keyMap{
		Up:       key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("k/↑", "up")),
		Down:     key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("j/↓", "down")),
		Left:     key.NewBinding(key.WithKeys("h", "left"), key.WithHelp("h/←", "pane left")),
		Right:    key.NewBinding(key.WithKeys("l", "right"), key.WithHelp("l/→", "pane right")),
		NextPane: key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next pane")),
		PrevPane: key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("S-tab", "prev pane")),
		Enter:    key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open")),
		Back:     key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		Top:      key.NewBinding(key.WithKeys("g", "home"), key.WithHelp("g", "top")),
		Bottom:   key.NewBinding(key.WithKeys("G", "end"), key.WithHelp("G", "bottom")),
		MarkRead: key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "mark read")),
		MarkAll:  key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "mark all visible read")),
		Undo:     key.NewBinding(key.WithKeys("u"), key.WithHelp("u", "undo mark read")),
		Help:     key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Quit:     key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	}
}
