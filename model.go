package main

import (
	tea "github.com/charmbracelet/bubbletea"
)

var _ tea.Model = (*model)(nil)

type model struct {
	w, h int // Track the size of the window
}

func (m *model) Init() tea.Cmd {
	return nil
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		return m, nil
	}
	return m, nil
}

func (m *model) View() string {
	return ""
}
